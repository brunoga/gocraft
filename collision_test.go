package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// worldWithWall returns a World holding a single loaded chunk with a solid
// vertical wall at x=wallX spanning the given y range (and z=wallZ). Cells that
// are not part of the wall read as empty (0), while unloaded chunks read as -1
// (obstacle), so tests must stay inside chunk {0,0,0} (coords 0..31).
func worldWithWall(t *testing.T, wallX, wallZ, y0, y1 int) *World {
	t.Helper()
	w := NewWorld()
	cid := Vec3{0, 0, 0}
	chunk := NewChunk(cid)
	const stone = 4 // IsObstacle(4) == true
	for y := y0; y <= y1; y++ {
		chunk.add(Vec3{wallX, y, wallZ}, stone)
	}
	w.storeChunk(cid, chunk)
	return w
}

// A fast move that jumps past a thin wall in a single frame must not tunnel
// through it: without stepping, Collide only inspects the destination cell.
func TestCollideSteppedBlocksTunneling(t *testing.T) {
	w := worldWithWall(t, 12, 10, 14, 18)

	from := mgl32.Vec3{9, 16, 10}
	to := mgl32.Vec3{15, 16, 10} // 6 blocks in one frame, straight through the wall

	got, _ := w.CollideStepped(from, to)
	if got.X() >= 12 {
		t.Fatalf("player tunneled through wall: got x=%.3f, want < 12", got.X())
	}

	// Sanity check that the un-stepped Collide *would* have tunneled, i.e. the
	// wall is genuinely invisible to a single destination-only check.
	naive, _ := w.Collide(to)
	if naive.X() < 12 {
		t.Fatalf("test setup wrong: single-step Collide already stopped at x=%.3f", naive.X())
	}
}

// For a move shorter than one step, CollideStepped must behave exactly like
// Collide (it delegates to it).
func TestCollideSteppedShortMoveMatchesCollide(t *testing.T) {
	w := worldWithWall(t, 12, 10, 14, 18)

	from := mgl32.Vec3{11.0, 16, 10}
	to := mgl32.Vec3{11.1, 16, 10}

	stepped, _ := w.CollideStepped(from, to)
	direct, _ := w.Collide(to)
	if stepped != direct {
		t.Fatalf("short move diverged: stepped=%v direct=%v", stepped, direct)
	}
}

// Falling onto a floor must report stop=true (used to zero fall velocity) and
// land the player just above the floor.
func TestCollideSteppedFloorStops(t *testing.T) {
	w := NewWorld()
	cid := Vec3{0, 0, 0}
	chunk := NewChunk(cid)
	const stone = 4
	// Floor slab at y=10 around x=10,z=10.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			chunk.add(Vec3{10 + dx, 10, 10 + dz}, stone)
		}
	}
	w.storeChunk(cid, chunk)

	from := mgl32.Vec3{10, 14, 10}
	to := mgl32.Vec3{10, 10.5, 10} // fall toward the floor

	got, stop := w.CollideStepped(from, to)
	if !stop {
		t.Fatalf("expected stop=true when landing on floor")
	}
	if got.Y() < 11 {
		t.Fatalf("player sank into floor: got y=%.3f, want >= 11", got.Y())
	}
}
