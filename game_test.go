package main

import (
	"math/rand/v2"
	"testing"
)

func testRNG() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

// emptyBoard: all land, no growth, unowned.
func emptyBoard() *Board {
	b := &Board{}
	for i := range b.Cells {
		b.Cells[i].Side = SideNone
		b.Cells[i].OldSide = SideNone
	}
	return b
}

func TestMoveSingleDirection(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Val[0] = 18
	c.Dirs[3] = true // right
	b.Tick(testRNG())
	// surplus/3 = 6 moves right (update.c:238 with move=3, one direction)
	if c.Val[0] != 12 {
		t.Fatalf("source = %d, want 12", c.Val[0])
	}
	d := b.at(6, 5)
	if d.Side != 0 || d.Val[0] != 6 {
		t.Fatalf("dest side=%d val=%d, want side=0 val=6", d.Side, d.Val[0])
	}
}

func TestMoveSplitsAcrossDirections(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Val[0] = 18
	c.Dirs[3] = true // right
	c.Dirs[2] = true // down
	b.Tick(testRNG())
	// Directions are processed sequentially with surplus recomputed (as in the
	// original): first gets 18/3/2 = 3, second 15/3/2 = 2.5 -> 2 or 3.
	r, d := b.at(6, 5).Val[0], b.at(5, 6).Val[0]
	if r < 2 || r > 3 || d < 2 || d > 3 {
		t.Fatalf("dests = %d,%d, want each in [2,3]", r, d)
	}
	if c.Val[0]+r+d != 18 {
		t.Fatalf("troops not conserved: %d+%d+%d != 18", c.Val[0], r, d)
	}
}

func TestMoveCappedAtMaxVal(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Val[0] = 18
	c.Dirs[3] = true
	d := b.at(6, 5)
	d.Side = 0
	d.Val[0] = MaxVal - 2
	b.Tick(testRNG())
	if d.Val[0] != MaxVal {
		t.Fatalf("dest = %d, want %d", d.Val[0], MaxVal)
	}
	if c.Val[0] != 16 { // only 2 moved despite surplus/3 = 6
		t.Fatalf("source = %d, want 16", c.Val[0])
	}
}

func TestSeaBlocksMovement(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Val[0] = 18
	c.Dirs[3] = true
	b.at(6, 5).Level = -1
	b.Tick(testRNG())
	if c.Val[0] != 18 || b.at(6, 5).Val[0] != 0 {
		t.Fatalf("troops crossed sea: src=%d dst=%d", c.Val[0], b.at(6, 5).Val[0])
	}
}

func TestEnemyEntryStartsFight(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Val[0] = 18
	c.Dirs[3] = true
	d := b.at(6, 5)
	d.Side = 1
	d.Val[1] = 5
	b.moveOut(c, 5+5*BoardW, 3, 1, testRNG()) // one direct move, no combat yet
	if c.Val[0] != 12 {
		t.Fatalf("attacker source = %d, want 12", c.Val[0])
	}
	if d.Side != SideFight || d.OldSide != 1 || d.Val[0] != 6 || d.Val[1] != 5 {
		t.Fatalf("dest side=%d old=%d v=%v, want contested 6v5", d.Side, d.OldSide, d.Val)
	}
}

func TestFightOutnumberedSideWipedAndCaptureClearsOrders(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = SideFight
	c.OldSide = 1
	c.Val[0] = 15
	c.Val[1] = 3
	c.Dirs[1] = true // defender's old order, must be wiped on capture
	// side1 ratio = 15/3 = 5: loss >= (25-1)*5 = 120 >= 3, always wiped.
	// side0 ratio = 0.2: loss <= (0.04-1+1.98)*5 ~ 5.1, never kills 15.
	b.fightCell(c, testRNG())
	if c.Side != 0 || c.Val[1] != 0 {
		t.Fatalf("side=%d val1=%d, want capture by 0", c.Side, c.Val[1])
	}
	if c.Dirs != [4]bool{} {
		t.Fatalf("orders not cleared on capture: %v", c.Dirs)
	}
	if c.Val[0] <= 15-7 || c.Val[0] > 15 {
		t.Fatalf("winner troops = %d, outside plausible attrition", c.Val[0])
	}
}

func TestGrowthRate(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	c.Growth = 100 // base: guaranteed +1 per tick
	rng := testRNG()
	for i := 0; i < 5; i++ {
		b.growCell(c, rng)
	}
	if c.Val[0] != 5 {
		t.Fatalf("base grew %d in 5 ticks, want 5", c.Val[0])
	}
	// ~50% town over many ticks
	c2 := b.at(7, 7)
	c2.Side = 1
	c2.Growth = 50
	n := 0
	for i := 0; i < 10000; i++ {
		c2.Val[1] = 0
		b.growCell(c2, rng)
		n += c2.Val[1]
	}
	if n < 4500 || n > 5500 {
		t.Fatalf("growth 50 produced %d/10000, want ~5000", n)
	}
	// unowned towns don't grow
	c3 := b.at(9, 9)
	c3.Growth = 90
	b.growCell(c3, rng)
	if c3.Val[0] != 0 || c3.Val[1] != 0 {
		t.Fatal("unowned town grew")
	}
}

func TestSetVectors(t *testing.T) {
	b := emptyBoard()
	c := b.at(5, 5)
	c.Side = 0
	b.SetVectors(0, 5, 5, [4]bool{true, false, false, true}, false)
	if !c.Dirs[0] || !c.Dirs[3] {
		t.Fatal("toggle on failed")
	}
	b.SetVectors(0, 5, 5, [4]bool{true, false, false, false}, false)
	if c.Dirs[0] || !c.Dirs[3] {
		t.Fatal("toggle off failed")
	}
	b.SetVectors(0, 5, 5, [4]bool{}, true) // center click: force clear
	if c.Dirs != [4]bool{} {
		t.Fatal("force clear failed")
	}
	b.SetVectors(1, 5, 5, [4]bool{true, true, true, true}, false) // not owner
	if c.Dirs != [4]bool{} {
		t.Fatal("enemy set vectors on foreign cell")
	}
}

func TestAliveSidesAndElimination(t *testing.T) {
	b := emptyBoard()
	alive := b.AliveSides()
	if alive[0] || alive[1] {
		t.Fatal("empty board has living sides")
	}
	b.at(1, 1).Side = 0
	b.at(1, 1).Val[0] = 1
	b.at(3, 3).Side = 1
	b.at(3, 3).Growth = 100 // 0 troops but producing: still alive
	alive = b.AliveSides()
	if !alive[0] || !alive[1] {
		t.Fatalf("alive = %v, want both", alive)
	}
	b.at(1, 1).Val[0] = 0
	alive = b.AliveSides()
	if alive[0] {
		t.Fatal("side 0 should be eliminated")
	}
}

func TestNewBoardValid(t *testing.T) {
	rng := testRNG()
	for i := 0; i < 50; i++ {
		b := NewBoard(rng)
		b0, b1 := b.at(2, 2), b.at(BoardW-3, BoardH-3)
		if b0.Side != 0 || b0.Growth != 100 || b0.Val[0] != MaxVal {
			t.Fatalf("bad base0: %+v", b0)
		}
		if b1.Side != 1 || b1.Growth != 100 || b1.Val[1] != MaxVal {
			t.Fatalf("bad base1: %+v", b1)
		}
		if !b.connected(2+2*BoardW, (BoardW-3)+(BoardH-3)*BoardW) {
			t.Fatal("bases not connected")
		}
	}
}
