# xbattle-web Implementation Plan

> **For agentic workers:** Execute inline (superpowers:executing-plans). Spec:
> `docs/superpowers/specs/2026-07-08-xbattle-web-design.md` — all rule formulas and
> C-source line refs live there; this plan does not repeat them.

**Goal:** Two-player web xbattle: name → queue → real-time game → data deleted at end.

**Architecture:** One Go binary. Matchmaker goroutine pairs queued WebSocket players;
one goroutine per game owns all board state (channel-fed, lock-free) and ticks every
500 ms. Vanilla-JS canvas client served from an embedded static file.

**Tech Stack:** Go stdlib + gorilla/websocket; HTML5 canvas, no build step.

## Global Constraints

- No persistence of any kind; game structs unreachable (GC-able) after game end.
- Rules and visual constants must match spec values taken from the C source.
- Thousands of concurrent games: no global locks in the tick path, per-game goroutines.
- ponytail: single package `main`, 4 source files, no config, no framework.

## File Structure

- `go.mod` / `go.sum` — module `xbattle-web`, dep gorilla/websocket.
- `game.go` — engine: `Cell`, `Board`, `NewBoard(rng)` (mapgen), `(*Board).Tick(rng)`,
  `(*Board).SetVectors(side, x, y, dirs [4]bool, force bool)`, `(*Board).AliveSides() [2]bool`.
  Pure — no goroutines, no networking. Constants from spec (maxval 20, move 3, fight 5…).
- `game_test.go` — engine checks: movement math, combat attrition, capture-clears-orders,
  growth probability, sea impassable, elimination detection.
- `main.go` — HTTP + WS: `player` struct, matchmaker goroutine, `runGame(p1, p2)` goroutine
  (ticker + command channel + disconnect), JSON protocol per spec, serves `static/`.
- `static/index.html` — all client HTML/CSS/JS inline: name form, waiting screen, canvas
  game (render per spec visuals; click-angle → direction vectors per spec geometry),
  end banner + requeue button.

### Task 1: Engine (`game.go` + `game_test.go`)

**Produces:** the API above; board serialization method `(*Board).Snapshot() []CellState`
(x omitted — index = y*W+x; fields: side, old-side, level, growth, val [2]int,
dir bitmask uint8).

- [ ] Write failing tests for: 1-dir move = surplus/3; 2-dir split /2; dest cap at 20;
      sea blocked; enemy entry → FIGHT holding both values; attrition kills outnumbered
      side within bounded ticks and winner-changed-hands has cleared dirs; town growth ≈
      growth/100 rate over many ticks; AliveSides false after wipe.
      Run: `go test ./...` → FAIL (undefined symbols).
- [ ] Implement engine per spec formulas. Run: `go test ./...` → PASS.
- [ ] Commit.

### Task 2: Server (`main.go`)

**Consumes:** Task 1 API. **Produces:** `GET /` static page, `GET /ws` WebSocket speaking
the spec protocol; matchmaker pairs players FIFO; game goroutine ends on win/disconnect,
notifies both, closes conns.

- [ ] Implement matchmaker + game loop + protocol.
- [ ] Verify: `go vet ./...`, `go build`, then a Go test (`main_test.go`) that dials two
      WS clients with names, receives `start` with opposite sides, receives `state` ticks,
      sends a `cmd`, sees vectors echoed in next state; closing one client yields `end`
      with the other as winner. Run: `go test ./...` → PASS.
- [ ] Commit.

### Task 3: Client (`static/index.html`)

**Consumes:** Task 2 protocol. **Produces:** playable UI.

- [ ] Implement screens, canvas renderer (troop squares, town arcs, vector arrows,
      FIGHT concentric tokens + X, grid, sea/land colors), click geometry (90° sectors
      offset 45°, 22.5° corner dual-vectors, center dead-zone clears; left toggle,
      right force; suppress context menu).
- [ ] Verify end-to-end: run server, open two browser sessions (preview tools), play
      until visible troop flow and a combat; screenshot check vs spec visuals.
- [ ] Commit.

### Task 4: Scale sanity + wrap-up

- [ ] `go test -race ./...` clean.
- [ ] Quick load check: Go test spawning 2000 in-process games (4000 fake ws or direct
      runGame with stub conns) for a few ticks without error; assert no leaked goroutines
      after games end (runtime.NumGoroutine back near baseline).
- [ ] README with run instructions. Commit.
