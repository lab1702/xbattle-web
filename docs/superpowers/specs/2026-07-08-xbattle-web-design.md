# xbattle-web — Design Spec (2026-07-08)

Web recreation of xbattle 5.4.1 (original C source in `xbattle-5.4.1/`, used as blueprint).
Scope decided with user: **classic core**, **square tiles**, **full visibility**.

## Product

- Player opens page, enters a name, joins the matchmaking queue.
- First two queued players are paired into a game (red vs cyan).
- Real-time concurrent play (no turns) until one side is eliminated or disconnects.
- No accounts, no scores; all game data is dropped from memory when the game ends.
- Server must support thousands of simultaneous games (goroutine per game).

## Stack

- Single Go binary: `net/http` static file server + WebSocket endpoint.
- Only dependency: `github.com/gorilla/websocket`.
- Client: one static HTML page, vanilla JS, `<canvas>` rendering. No framework, no build step.

## Server architecture

- **Matchmaker goroutine**: owns the waiting slot; receives joined players over a channel;
  pairs consecutive arrivals and spawns a game goroutine.
- **Game goroutine**: sole owner of board state. Loop over `time.Ticker` (500 ms) and a
  command channel fed by the two per-connection reader goroutines. No locks on game state.
- **End of game**: broadcast result, close sockets, return — GC reclaims everything.
  Disconnect = opponent wins.

## Game rules (from C source; file:line refs are into xbattle-5.4.1/)

Defaults: board 15×15, maxval 20, tick 0.5 s (`-speed 5` → 25000/speed → 0.5 s, parse.c:912,
main.c:110), move 3, fight 5.

- **Cell state**: per-side troop `value[2]`, per-direction `dir[4]` (0=up,1=left,2=down,3=right),
  `side` (none / 0 / 1 / FIGHT), `level` (<0 = sea, impassable), `growth`, `lowbound` (unused v1, 0).
- **Tick** (update.c:26): visit all cells in a fresh random permutation. Per cell:
  growth → fight (if FIGHT) → movement (directions iterated from a rotating start index).
  Movement is single-buffered (reads live neighbor state).
- **Growth** (update.c:479): if owned, growth>0, value<maxval: `value++` with probability
  growth/100 per tick.
- **Movement** (update.c:196): per active direction,
  `move = surplus * (1/3) * (1/moveCount)`; integer part moves, fractional remainder moves
  1 troop with that probability; skip sea; cap destination at maxval.
  Into empty → capture; friendly → merge; enemy → cell becomes FIGHT (keep `old_side`);
  fighting → reinforce.
- **Combat** (update.c:609): per side, `versus = enemy troops`, `ratio = versus/value`,
  `loss = round((ratio² − 1 + 0.02·rand(100)) · 5)`, floored at 0; wiped if loss ≥ value.
  One survivor → takes cell; if it changed hands, clear all vectors/orders (update.c:1142).
  Zero survivors → empty. Multiple → stays FIGHT.
- **Map gen**: ~8 towns (growth 50–99, unowned), a few sea patches, one base per side
  (growth 100, value 20) in opposite corners (TOWN_MIN/MAX constant.h:112, base init.c:565).
- **Victory** (added — original has none): a side with zero troops in all cells loses.

## Client

- Canvas, 45-px cells, 44-px pitch (1-px shared black grid line), gray page border.
- Colors (parse.h): land rgb(210,220,150), sea rgb(70,132,200)→rgb(30,65,185),
  red rgb(255,0,0), cyan rgb(100,255,210), grid black, border rgb(192,192,192).
- Troops: filled square centered in cell, size linear in count from max(2, 5%) to 90% of
  cell (shape.c:448). FIGHT cell: both sides' tokens concentric (larger behind) + battle X.
- Towns: 2-px ring arc, radius linear in growth over [50,100] from 20% to 85% of half-cell,
  owner's color or black if unowned. Base = full circle (growth 100).
- Vectors: arrow (shaft + 2 barbs) from center toward each active direction, half length
  when cell has 0 troops; own vectors always visible, opponent's too (full visibility).
- **Input** (shape.c:493, main.c:369): left-click sets vectors on an owned cell by angle of
  click relative to cell center; 90° sectors rotated 45° → side sectors set one vector,
  22.5° corner spans set the two adjacent vectors; center dead-zone (r ≈ 7 px) clears all.
  Left = toggle, right/middle = force-set (clear then set). Clicks on non-owned cells ignored.
- **Protocol** (JSON over WS): client→server `{"join": name}`, `{"cmd": {x, y, dirs: [..], force: bool}}`;
  server→client `{"wait": true}`, `{"start": {side, names, board-meta}}`, per-tick
  `{"state": cells}` (full board — 225 cells is small), `{"end": {winner, reason}}`.

## Skipped (add later without structural change)

Hills/forest/digin modifiers, fog (-horizon/-hidden), march/attack/dig/fill/build/scuttle/
artillery/paratroops/reserve, erosion, decay, reconnection, spectators, hex tiling.
