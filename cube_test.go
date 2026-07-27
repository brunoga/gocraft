package main

import "testing"

func TestVertexAO(t *testing.T) {
	cases := []struct {
		s1, s2, c bool
		want      float32
	}{
		{false, false, false, 3}, // fully open
		{true, false, false, 2},  // one edge
		{false, true, false, 2},  // other edge
		{false, false, true, 2},  // diagonal only
		{true, false, true, 1},   // edge + diagonal
		{true, true, false, 0},   // both edges -> fully dark regardless of diagonal
		{true, true, true, 0},
	}
	for _, c := range cases {
		if got := vertexAO(c.s1, c.s2, c.c); got != c.want {
			t.Errorf("vertexAO(%v,%v,%v) = %v, want %v", c.s1, c.s2, c.c, got, c.want)
		}
	}
}

// occAt builds an occ function that reports true only for the listed offsets.
func occAt(offsets ...[3]int) func(dx, dy, dz int) bool {
	set := map[[3]int]bool{}
	for _, o := range offsets {
		set[o] = true
	}
	return func(dx, dy, dz int) bool { return set[[3]int{dx, dy, dz}] }
}

func TestFaceAO_UpFace(t *testing.T) {
	// The up face (+Y) samples the layer at y+1, in-plane axes X,Z. Emission
	// order is v0(-x,+z) v1(+x,+z) v2(+x,-z) v3=v2 v4(-x,-z) v5=v0.

	// An edge occluder on the +X side of the layer above darkens both +X
	// corners (level 2) and leaves the -X corners open (level 3).
	got := faceAO(occAt([3]int{1, 1, 0}), sup)
	want := [6]float32{3, 2, 2, 2, 3, 3}
	if got != want {
		t.Errorf("edge occluder: faceAO(up) = %v, want %v", got, want)
	}

	// A diagonal-only occluder at (+x,+y,+z) darkens just the (+X,+Z) corner
	// (v1) to level 2.
	got = faceAO(occAt([3]int{1, 1, 1}), sup)
	want = [6]float32{3, 2, 3, 3, 3, 3}
	if got != want {
		t.Errorf("diagonal occluder: faceAO(up) = %v, want %v", got, want)
	}

	// Both edges around the (+X,+Z) corner -> that corner is fully dark (0).
	got = faceAO(occAt([3]int{1, 1, 0}, [3]int{0, 1, 1}), sup)
	if got[1] != 0 {
		t.Errorf("both-edge corner v1 = %v, want 0 (full dark)", got[1])
	}

	// No occluders anywhere -> all open.
	got = faceAO(occAt(), sup)
	want = [6]float32{3, 3, 3, 3, 3, 3}
	if got != want {
		t.Errorf("open face: faceAO(up) = %v, want %v", got, want)
	}
}

// Every corner must sample only blocks in the layer directly in front of its
// face (one step along the normal). A block in the face's own layer or behind
// it must never affect that face's AO.
func TestFaceAO_OnlySamplesFrontLayer(t *testing.T) {
	// For the up face the front layer is y=+1. Occluders at y=0 and y=-1 must
	// be ignored.
	got := faceAO(occAt([3]int{1, 0, 0}, [3]int{1, -1, 0}), sup)
	want := [6]float32{3, 3, 3, 3, 3, 3}
	if got != want {
		t.Errorf("up face must ignore non-front layers: got %v, want %v", got, want)
	}
}
