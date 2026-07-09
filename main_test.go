package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	go matchmaker()
	m.Run()
}

type srvMsg struct {
	Wait  bool `json:"wait"`
	Start *struct {
		Side  int       `json:"side"`
		Names [2]string `json:"names"`
	} `json:"start"`
	State []CellState `json:"state"`
	End   *struct {
		Winner int    `json:"winner"`
		Reason string `json:"reason"`
	} `json:"end"`
}

func dialPlayer(t *testing.T, url, name string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(clientMsg{Join: name}); err != nil {
		t.Fatal(err)
	}
	return c
}

func readUntil(t *testing.T, c *websocket.Conn, pred func(srvMsg) bool) srvMsg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m srvMsg
		if err := c.ReadJSON(&m); err != nil {
			t.Fatalf("read: %v", err)
		}
		if pred(m) {
			return m
		}
	}
	t.Fatal("timed out waiting for message")
	return srvMsg{}
}

func TestMatchmakingGameFlowAndDisconnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(serveWS))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") // ws://host

	alice := dialPlayer(t, url, "alice")
	defer alice.Close()
	bob := dialPlayer(t, url, "bob")
	defer bob.Close()

	sa := readUntil(t, alice, func(m srvMsg) bool { return m.Start != nil })
	sb := readUntil(t, bob, func(m srvMsg) bool { return m.Start != nil })
	if sa.Start.Side == sb.Start.Side {
		t.Fatalf("both players got side %d", sa.Start.Side)
	}
	if sa.Start.Names != [2]string{"alice", "bob"} {
		t.Fatalf("names = %v", sa.Start.Names)
	}

	// state ticks arrive and have the right shape
	st := readUntil(t, alice, func(m srvMsg) bool { return m.State != nil })
	if len(st.State) != BoardW*BoardH {
		t.Fatalf("state has %d cells", len(st.State))
	}

	// command: alice sets a vector on her base; it shows up in a later state
	baseIdx := 2 + 2*BoardW
	if sa.Start.Side == 1 {
		baseIdx = (BoardW - 3) + (BoardH-3)*BoardW
	}
	alice.WriteJSON(clientMsg{Cmd: &cmdMsg{X: baseIdx % BoardW, Y: baseIdx / BoardW,
		Dirs: [4]bool{false, false, false, true}}})
	readUntil(t, alice, func(m srvMsg) bool {
		return m.State != nil && m.State[baseIdx].D&(1<<3) != 0
	})

	// disconnect: bob drops, alice wins
	bob.Close()
	end := readUntil(t, alice, func(m srvMsg) bool { return m.End != nil })
	if end.End.Winner != sa.Start.Side || end.End.Reason != "disconnect" {
		t.Fatalf("end = %+v, want alice by disconnect", end.End)
	}
}

func TestQueuedPlayerDisconnectDiscarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(serveWS))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	ghost := dialPlayer(t, url, "ghost")
	readUntil(t, ghost, func(m srvMsg) bool { return m.Wait })
	ghost.Close()
	time.Sleep(100 * time.Millisecond) // let the reader notice

	// next two arrivals should be paired with each other, not the ghost
	p1 := dialPlayer(t, url, "p1")
	defer p1.Close()
	p2 := dialPlayer(t, url, "p2")
	defer p2.Close()
	s := readUntil(t, p1, func(m srvMsg) bool { return m.Start != nil })
	if s.Start.Names != [2]string{"p1", "p2"} {
		t.Fatalf("names = %v, ghost not discarded", s.Start.Names)
	}
}
