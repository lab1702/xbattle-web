package main

// HTTP + WebSocket server: matchmaker pairs queued players, one goroutine per
// game owns all state. Nothing is persisted; when a game ends its struct is
// dropped and GC'd.

import (
	"embed"
	"io/fs"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static
var staticFS embed.FS

const TickInterval = 500 * time.Millisecond // -speed 5 (parse.c:912, main.c:110)

type player struct {
	name string
	conn wsConn
	cmds chan cmdMsg // closed by reader on disconnect
}

// wsConn is the slice of *websocket.Conn the game needs; lets tests stub it.
type wsConn interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}

type cmdMsg struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Dirs  [4]bool `json:"dirs"`
	Force bool    `json:"force"`
}

type clientMsg struct {
	Join string  `json:"join,omitempty"`
	Cmd  *cmdMsg `json:"cmd,omitempty"`
}

var queue = make(chan *player)

// matchmaker pairs consecutive arrivals. If the waiting player's socket dies
// while queued, its cmds channel closes and we discard it.
func matchmaker() {
	var waiting *player
	for p := range queue {
		if waiting == nil {
			waiting = p
			p.conn.WriteJSON(map[string]any{"wait": true})
			continue
		}
		alive := true
		select {
		case _, ok := <-waiting.cmds: // early cmds are meaningless pre-game, drop
			alive = ok
		default:
		}
		if !alive { // waiter disconnected while queued
			waiting.conn.Close()
			waiting = p
			p.conn.WriteJSON(map[string]any{"wait": true})
			continue
		}
		go runGame([2]*player{waiting, p})
		waiting = nil
	}
}

// runGame owns the board for its whole life. Sole writer: no locks.
func runGame(p [2]*player) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	board := NewBoard(rng)
	names := [2]string{p[0].name, p[1].name}
	for s := 0; s < 2; s++ {
		p[s].conn.WriteJSON(map[string]any{"start": map[string]any{
			"side": s, "names": names, "w": BoardW, "h": BoardH, "maxval": MaxVal,
		}})
	}
	defer func() {
		for s := 0; s < 2; s++ {
			p[s].conn.Close()
		}
	}()

	end := func(winner int, reason string) {
		msg := map[string]any{"end": map[string]any{"winner": winner, "reason": reason}}
		p[0].conn.WriteJSON(msg)
		p[1].conn.WriteJSON(msg)
	}

	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()
	for {
		select {
		case c, ok := <-p[0].cmds:
			if !ok {
				end(1, "disconnect")
				return
			}
			board.SetVectors(0, c.X, c.Y, c.Dirs, c.Force)
		case c, ok := <-p[1].cmds:
			if !ok {
				end(0, "disconnect")
				return
			}
			board.SetVectors(1, c.X, c.Y, c.Dirs, c.Force)
		case <-ticker.C:
			board.Tick(rng)
			state := map[string]any{"state": board.Snapshot()}
			e0 := p[0].conn.WriteJSON(state)
			e1 := p[1].conn.WriteJSON(state)
			if e0 != nil || e1 != nil {
				// write failure: reader will close cmds shortly; let that path end it
				continue
			}
			alive := board.AliveSides()
			if !alive[0] || !alive[1] {
				switch {
				case alive[0]:
					end(0, "elimination")
				case alive[1]:
					end(1, "elimination")
				default:
					end(-1, "mutual destruction")
				}
				return
			}
		}
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// serveWS: first message must be {"join": name}; then a reader goroutine
// forwards cmd messages to the game and closes cmds on disconnect.
func serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	var m clientMsg
	if err := conn.ReadJSON(&m); err != nil || m.Join == "" {
		conn.Close()
		return
	}
	name := m.Join
	if len(name) > 24 {
		name = name[:24]
	}
	p := &player{name: name, conn: conn, cmds: make(chan cmdMsg, 8)}
	go func() {
		defer close(p.cmds)
		for {
			var m clientMsg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Cmd != nil {
				select { // drop rather than block if the game is gone/slow
				case p.cmds <- *m.Cmd:
				default:
				}
			}
		}
	}()
	queue <- p
}

func main() {
	static, _ := fs.Sub(staticFS, "static")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/ws", serveWS)
	go matchmaker()
	log.Println("xbattle-web listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
