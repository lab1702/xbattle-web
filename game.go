package main

// Engine port of xbattle 5.4.1 classic core. Formula sources are cited in
// docs/superpowers/specs/2026-07-08-xbattle-web-design.md as file:line into
// the original C source (xbattle-5.4.1/).

import "math/rand/v2"

const (
	BoardW   = 15
	BoardH   = 15
	MaxVal   = 20  // -maxval
	MoveDiv  = 3.0 // -move
	FightK   = 5.0 // -fight
	TownMin  = 50  // TOWN_MIN
	TownMax  = 100 // TOWN_MAX
	NumTowns = 8
	NumSeas  = 4 // sea blob seeds
)

// Cell.Side values (0 and 1 are the players).
const (
	SideNone  int8 = -1
	SideFight int8 = 2
)

// Direction indices match constant.h:231: 0=up 1=left 2=down 3=right.
var dirDX = [4]int{0, -1, 0, 1}
var dirDY = [4]int{-1, 0, 1, 0}

type Cell struct {
	Level   int8 // <0 = sea (impassable)
	Growth  uint8
	Side    int8
	OldSide int8 // owner before a fight started
	Val     [2]int
	Dirs    [4]bool
}

type Board struct {
	Cells [BoardW * BoardH]Cell
}

func (b *Board) at(x, y int) *Cell {
	return &b.Cells[y*BoardW+x]
}

func (b *Board) neighbor(i, dir int) *Cell {
	x, y := i%BoardW+dirDX[dir], i/BoardW+dirDY[dir]
	if x < 0 || x >= BoardW || y < 0 || y >= BoardH {
		return nil
	}
	return b.at(x, y)
}

// NewBoard generates a map: sea blobs, neutral towns, one base per side in
// opposite corners. Regenerates until the bases are land-connected.
func NewBoard(rng *rand.Rand) *Board {
	for {
		b := &Board{}
		for i := range b.Cells {
			b.Cells[i].Side = SideNone
			b.Cells[i].OldSide = SideNone
		}
		for s := 0; s < NumSeas; s++ { // random walk blobs, init.c-style flavor
			x, y := 2+rng.IntN(BoardW-4), 2+rng.IntN(BoardH-4)
			for n := 0; n < 4; n++ {
				b.at(x, y).Level = -1
				d := rng.IntN(4)
				x = clamp(x+dirDX[d], 0, BoardW-1)
				y = clamp(y+dirDY[d], 0, BoardH-1)
			}
		}
		for t := 0; t < NumTowns; t++ { // init_towns init.c:384, growth 50..99
			c := b.at(rng.IntN(BoardW), rng.IntN(BoardH))
			if c.Level < 0 || c.Growth > 0 {
				continue
			}
			c.Growth = uint8(TownMin + rng.IntN(TownMax-TownMin))
		}
		for s, pos := range [2][2]int{{2, 2}, {BoardW - 3, BoardH - 3}} {
			c := b.at(pos[0], pos[1]) // base: growth 100, pre-occupied (init.c:565)
			c.Level = 0
			c.Growth = 100
			c.Side = int8(s)
			c.Val[s] = MaxVal
		}
		if b.connected(2+2*BoardW, (BoardW-3)+(BoardH-3)*BoardW) {
			return b
		}
	}
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func (b *Board) connected(from, to int) bool {
	var seen [BoardW * BoardH]bool
	seen[from] = true
	queue := []int{from}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		if i == to {
			return true
		}
		for d := 0; d < 4; d++ {
			x, y := i%BoardW+dirDX[d], i/BoardW+dirDY[d]
			if x < 0 || x >= BoardW || y < 0 || y >= BoardH {
				continue
			}
			j := y*BoardW + x
			if !seen[j] && b.Cells[j].Level >= 0 {
				seen[j] = true
				queue = append(queue, j)
			}
		}
	}
	return false
}

// Tick runs one 0.5s update, mirroring update_board (update.c:26): cells in a
// fresh random permutation; per cell growth -> fight -> movement, directions
// iterated from a rotating random start. Single-buffered like the original.
func (b *Board) Tick(rng *rand.Rand) {
	for _, i := range rng.Perm(len(b.Cells)) {
		c := &b.Cells[i]
		b.growCell(c, rng)
		if c.Side == SideFight {
			b.fightCell(c, rng)
		}
		moves := 0
		for _, on := range c.Dirs {
			if on {
				moves++
			}
		}
		if moves == 0 {
			continue
		}
		start := rng.IntN(4)
		for k := 0; k < 4; k++ {
			d := (start + k) % 4
			if c.Dirs[d] {
				b.moveOut(c, i, d, moves, rng)
			}
		}
	}
}

// growCell: update_cell_growth (update.c:479). growth = % chance of +1/tick.
func (b *Board) growCell(c *Cell, rng *rand.Rand) {
	if c.Side != 0 && c.Side != 1 {
		return
	}
	s := c.Side
	if c.Growth > 0 && c.Val[s] < MaxVal && int(c.Growth) > rng.IntN(100) {
		c.Val[s]++
	}
}

// moveOut: update_cell (update.c:196) for one active direction.
func (b *Board) moveOut(c *Cell, i, dir, moves int, rng *rand.Rand) {
	dst := b.neighbor(i, dir)
	if dst == nil || dst.Level < 0 { // sea/edge blocks (update.c:215)
		return
	}
	side := c.Side
	if side == SideFight {
		side = c.OldSide // update.c:227: only into empty or own cells
		if side == SideNone || (dst.Side != SideNone && dst.Side != side) {
			return
		}
	}
	if side != 0 && side != 1 {
		return
	}
	surplus := c.Val[side] // lowbound always 0 in v1
	if surplus <= 0 {
		return
	}
	// update.c:238: surplus * move_hinder(=1/3) * move_moves(=1/moves)
	f := float64(surplus) / MoveDiv / float64(moves)
	n := int(f)
	if n > surplus {
		n = surplus
	}
	if n == 0 && f > 0 && rng.IntN(100) < int(100*f) { // probabilistic single troop
		n = 1
	}
	if n+dst.Val[side] > MaxVal { // destination cap
		n = MaxVal - dst.Val[side]
	}
	if n <= 0 {
		return
	}
	c.Val[side] -= n
	switch dst.Side {
	case SideNone:
		dst.Side = side
		dst.Val[side] += n
	case side, SideFight:
		dst.Val[side] += n
	default: // enemy: cell becomes contested (update.c:290)
		dst.OldSide = dst.Side
		dst.Side = SideFight
		dst.Val[side] += n
	}
}

// fightCell: update_cell_fight (update.c:609).
func (b *Board) fightCell(c *Cell, rng *rand.Rand) {
	versus := [2]int{c.Val[1], c.Val[0]}
	survivors, last := 0, SideNone
	for s := 0; s < 2; s++ {
		if c.Val[s] == 0 {
			continue
		}
		ratio := float64(versus[s]) / float64(c.Val[s])
		loss := (ratio*ratio - 1.0 + 0.02*float64(rng.IntN(100))) * FightK
		if loss < 0 {
			loss = 0
		}
		li := int(loss + 0.5)
		if li < c.Val[s] {
			c.Val[s] -= li
			survivors++
			last = int8(s)
		} else {
			c.Val[s] = 0
		}
	}
	switch survivors {
	case 1:
		c.Side = last
		if last != c.OldSide { // captured: orders wiped (update_cell_clean, update.c:1142)
			c.Dirs = [4]bool{}
		}
		c.OldSide = SideNone
	case 0:
		c.Side = SideNone
		c.OldSide = SideNone
		c.Dirs = [4]bool{}
	}
}

// SetVectors applies a player click: force clears then sets, otherwise each
// given direction toggles (set_move/set_move_force, update.c:1222,1291).
// Legal only on the player's own cell, or a contested cell they owned.
func (b *Board) SetVectors(side int8, x, y int, dirs [4]bool, force bool) {
	if x < 0 || x >= BoardW || y < 0 || y >= BoardH {
		return
	}
	c := b.at(x, y)
	if !(c.Side == side || (c.Side == SideFight && c.OldSide == side)) {
		return
	}
	if force {
		c.Dirs = dirs
		return
	}
	for d, on := range dirs {
		if on {
			c.Dirs[d] = !c.Dirs[d]
		}
	}
}

// AliveSides: a side is alive while it has any troops or any producing cell.
// (The original has no end check; spec adds elimination.)
func (b *Board) AliveSides() (alive [2]bool) {
	for i := range b.Cells {
		c := &b.Cells[i]
		for s := 0; s < 2; s++ {
			if c.Val[s] > 0 || (c.Side == int8(s) && c.Growth > 0) {
				alive[s] = true
			}
		}
	}
	return
}

// CellState is the per-cell wire format; board is sent as a flat array,
// index = y*BoardW + x.
type CellState struct {
	S int8   `json:"s"`           // side: -1 none, 0, 1, 2 fight
	O int8   `json:"o"`           // pre-fight owner (arrows' color while contested)
	L int8   `json:"l"`           // level (<0 sea)
	G uint8  `json:"g"`           // growth
	V [2]int `json:"v"`           // troops per side
	D uint8  `json:"d,omitempty"` // dir bitmask, bit i = direction i
}

func (b *Board) Snapshot() []CellState {
	out := make([]CellState, len(b.Cells))
	for i := range b.Cells {
		c := &b.Cells[i]
		var mask uint8
		for d, on := range c.Dirs {
			if on {
				mask |= 1 << d
			}
		}
		out[i] = CellState{S: c.Side, O: c.OldSide, L: c.Level, G: c.Growth, V: c.Val, D: mask}
	}
	return out
}
