# xbattle-web

Web recreation of [xbattle 5.4.1](xbattle-5.4.1/) (Lehar/Lesher, 1991-95).
Two players queue by name and fight in real time on a 15×15 board. No
accounts, no persistence — a finished game's data is gone.

## Run

```
docker compose up
```

Open http://localhost:8086 in two browsers, enter names, play.

For local development (needs Go 1.26): `go run .`

## Play

- Troops flow along direction vectors, ~⅓ of a cell's troops per direction
  per 0.5 s tick. Cells cap at 20.
- **Left-click** in a cell you own toggles the vector for that side of the
  cell; corner clicks toggle both adjacent vectors; clicking the center
  clears all; **right-click** sets exactly that vector.
- Rings are towns: occupy them and they produce troops (your base is a
  full-strength town). Blue cells are sea — impassable.
- Entering an enemy cell starts a fight (concentric tokens + X); the
  outnumbered side loses troops superlinearly. Eliminate the enemy to win.
  Disconnecting forfeits.

## Design

One goroutine per game owns all its state (no locks); a matchmaker goroutine
pairs arrivals; gorilla/websocket is the only dependency. Rules and visuals
are ported from the original C source — see
`docs/superpowers/specs/2026-07-08-xbattle-web-design.md` for the mapping
with file:line references, and `go test ./...` for the rule checks plus a
2000-concurrent-game load test.

## License

This is a derivative of xbattle, Copyright (C) 1995 Steve Lehar and Greg
Lesher, whose unmodified source is included in `xbattle-5.4.1/`. The whole
repository is distributed under the same terms — see
[xbattle-5.4.1/COPYRIGHT](xbattle-5.4.1/COPYRIGHT). The web port
(`*.go`, `static/`) was written in 2026.
