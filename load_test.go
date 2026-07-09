package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubConn struct{ writes atomic.Int64 }

func (s *stubConn) WriteJSON(any) error { s.writes.Add(1); return nil }
func (s *stubConn) ReadJSON(any) error  { return nil } // unused by runGame
func (s *stubConn) Close() error        { return nil }

// TestThousandsOfConcurrentGames: 2000 games tick concurrently, then all end
// by disconnect; every goroutine and game struct must be gone afterwards.
func TestThousandsOfConcurrentGames(t *testing.T) {
	const games = 2000
	base := runtime.NumGoroutine()
	var wg sync.WaitGroup
	firsts := make([]*player, games)
	conns := make([]*stubConn, games)
	for i := 0; i < games; i++ {
		a := &player{name: "a", conn: &stubConn{}, cmds: make(chan cmdMsg)}
		b := &player{name: "b", conn: &stubConn{}, cmds: make(chan cmdMsg)}
		firsts[i], conns[i] = a, a.conn.(*stubConn)
		wg.Add(1)
		go func() { defer wg.Done(); runGame([2]*player{a, b}) }()
	}
	time.Sleep(1200 * time.Millisecond) // at least two ticks for everyone
	for _, p := range firsts {
		close(p.cmds) // disconnect player 0 of every game
	}
	wg.Wait()
	for i, c := range conns {
		if c.writes.Load() < 3 { // start + >=2 states (+ end)
			t.Fatalf("game %d only wrote %d messages", i, c.writes.Load())
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > base+10 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if g := runtime.NumGoroutine(); g > base+10 {
		t.Fatalf("goroutines leaked: %d -> %d", base, g)
	}
}
