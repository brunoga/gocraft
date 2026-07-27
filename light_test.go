package main

import "testing"

// fakeGrid is an in-memory lightGrid over an inclusive [lo,hi] box.
type fakeGrid struct {
	lo, hi Vec3
	solid  map[Vec3]bool
	lit    map[Vec3]uint8
}

func newFakeGrid(lo, hi Vec3) *fakeGrid {
	return &fakeGrid{lo: lo, hi: hi, solid: map[Vec3]bool{}, lit: map[Vec3]uint8{}}
}

func (f *fakeGrid) blocksLight(p Vec3) bool { return f.solid[p] }
func (f *fakeGrid) loaded(p Vec3) bool {
	return p.X >= f.lo.X && p.X <= f.hi.X &&
		p.Y >= f.lo.Y && p.Y <= f.hi.Y &&
		p.Z >= f.lo.Z && p.Z <= f.hi.Z
}
func (f *fakeGrid) light(p Vec3) uint8 { return f.lit[p] }
func (f *fakeGrid) setLight(p Vec3, v uint8) {
	if v == 0 {
		delete(f.lit, p)
	} else {
		f.lit[p] = v
	}
}

func (f *fakeGrid) setSolid(p Vec3)   { f.solid[p] = true }
func (f *fakeGrid) clearSolid(p Vec3) { delete(f.solid, p) }

// fullRelight seeds skylight for every column and floods the whole grid.
func (f *fakeGrid) fullRelight() {
	f.lit = map[Vec3]uint8{}
	var q []Vec3
	for x := f.lo.X; x <= f.hi.X; x++ {
		for z := f.lo.Z; z <= f.hi.Z; z++ {
			q = append(q, seedSkyColumn(f, x, z, f.hi.Y, nil)...)
		}
	}
	propagateIncrease(f, q, nil)
}

func TestSkyOpenColumnFullyLit(t *testing.T) {
	g := newFakeGrid(Vec3{0, 0, 0}, Vec3{3, 3, 3})
	g.fullRelight()
	for x := 0; x <= 3; x++ {
		for y := 0; y <= 3; y++ {
			for z := 0; z <= 3; z++ {
				if got := g.light(Vec3{x, y, z}); got != MaxLight {
					t.Fatalf("open cell (%d,%d,%d) = %d, want %d", x, y, z, got, MaxLight)
				}
			}
		}
	}
}

// tunnelGrid builds a horizontal 1-high tunnel at y=0 for x in [0,w], lit only
// by an open vertical shaft at x=0. The tunnel is roofed (solid at y=1) for
// x>=1 so cells there get light only by horizontal spreading from the shaft.
func tunnelGrid(w int) *fakeGrid {
	g := newFakeGrid(Vec3{0, 0, 0}, Vec3{w, 2, 0})
	for x := 1; x <= w; x++ {
		g.setSolid(Vec3{x, 1, 0}) // roof
		g.setSolid(Vec3{x, 2, 0})
	}
	g.fullRelight()
	return g
}

func TestTunnelAttenuates(t *testing.T) {
	g := tunnelGrid(6)
	for x := 0; x <= 6; x++ {
		want := uint8(MaxLight - x) // 15 at the shaft, -1 per step
		if got := g.light(Vec3{x, 0, 0}); got != want {
			t.Errorf("tunnel x=%d light = %d, want %d", x, got, want)
		}
	}
}

func TestSealedRoomIsDark(t *testing.T) {
	g := newFakeGrid(Vec3{0, 0, 0}, Vec3{2, 2, 2})
	// Everything solid except the fully-enclosed centre cell.
	for x := 0; x <= 2; x++ {
		for y := 0; y <= 2; y++ {
			for z := 0; z <= 2; z++ {
				if !(x == 1 && y == 1 && z == 1) {
					g.setSolid(Vec3{x, y, z})
				}
			}
		}
	}
	g.fullRelight()
	if got := g.light(Vec3{1, 1, 1}); got != 0 {
		t.Fatalf("sealed cell = %d, want 0 (dark)", got)
	}
}

func TestPlaceBlockRemovesLightBeyond(t *testing.T) {
	g := tunnelGrid(6)
	// Place a block mid-tunnel; cells past it lose their only light path.
	p := Vec3{3, 0, 0}
	g.setSolid(p)
	lightPlaceBlock(g, p, nil)

	if got := g.light(Vec3{2, 0, 0}); got != MaxLight-2 {
		t.Errorf("before block: x=2 = %d, want %d", got, MaxLight-2)
	}
	for x := 3; x <= 6; x++ {
		if got := g.light(Vec3{x, 0, 0}); got != 0 {
			t.Errorf("past block: x=%d = %d, want 0", x, got)
		}
	}
}

func TestRemoveBlockRelightsBeyond(t *testing.T) {
	g := tunnelGrid(6)
	p := Vec3{3, 0, 0}
	g.setSolid(p)
	lightPlaceBlock(g, p, nil)

	// Dig it back out; the tunnel beyond should light up again.
	g.clearSolid(p)
	lightRemoveBlock(g, p, g.hi.Y, nil)

	for x := 0; x <= 6; x++ {
		want := uint8(MaxLight - x)
		if got := g.light(Vec3{x, 0, 0}); got != want {
			t.Errorf("after dig: x=%d = %d, want %d", x, got, want)
		}
	}
}

// Placing a block at the top of a shaft must remove skylight from the whole
// column below it (the sky-cut case), and digging it back must restore it.
func TestPlaceRemoveSkyColumn(t *testing.T) {
	g := newFakeGrid(Vec3{0, 0, 0}, Vec3{2, 3, 0})
	for y := 0; y <= 3; y++ {
		g.setSolid(Vec3{0, y, 0}) // left wall
		g.setSolid(Vec3{2, y, 0}) // right wall
	}
	g.fullRelight()
	for y := 0; y <= 3; y++ {
		if got := g.light(Vec3{1, y, 0}); got != MaxLight {
			t.Fatalf("shaft (1,%d) = %d, want %d", y, got, MaxLight)
		}
	}

	// Cap the shaft.
	cap := Vec3{1, 3, 0}
	g.setSolid(cap)
	lightPlaceBlock(g, cap, nil)
	for y := 0; y <= 2; y++ {
		if got := g.light(Vec3{1, y, 0}); got != 0 {
			t.Errorf("capped shaft (1,%d) = %d, want 0", y, got)
		}
	}

	// Re-open it.
	g.clearSolid(cap)
	lightRemoveBlock(g, cap, g.hi.Y, nil)
	for y := 0; y <= 3; y++ {
		if got := g.light(Vec3{1, y, 0}); got != MaxLight {
			t.Errorf("reopened shaft (1,%d) = %d, want %d", y, got, MaxLight)
		}
	}
}
