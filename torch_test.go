package main

import "testing"

// TestTorchOrientation checks that a torch mounts correctly to the clicked
// surface and reports the right supporting block, so removing that block can
// remove the torch.
func TestTorchOrientation(t *testing.T) {
	wall := Vec3{5, 10, 5}
	cases := []struct {
		cell Vec3
		want int
	}{
		{Vec3{6, 10, 5}, blockTorchXp}, // torch on the +X side of the wall
		{Vec3{4, 10, 5}, blockTorchXn},
		{Vec3{5, 10, 6}, blockTorchZp},
		{Vec3{5, 10, 4}, blockTorchZn},
		{Vec3{5, 11, 5}, blockTorch}, // torch on top of a floor block -> upright
	}
	for _, c := range cases {
		got := orientTorch(wall, c.cell)
		if got != c.want {
			t.Errorf("orientTorch(%v,%v)=%d want %d", wall, c.cell, got, c.want)
		}
		if !isTorch(got) {
			t.Errorf("orientTorch produced non-torch type %d", got)
		}
		if blockEmission(got) != torchLight {
			t.Errorf("torch variant %d should emit torchLight, got %d", got, blockEmission(got))
		}
		// The torch's support is the wall it hangs on (or the floor below an
		// upright one) -- exactly the block whose removal should drop it.
		wantSup := wall
		if got == blockTorch {
			wantSup = c.cell.Down()
		}
		if sup := torchSupport(c.cell, got); sup != wantSup {
			t.Errorf("torchSupport(%v,%d)=%v want %v", c.cell, got, sup, wantSup)
		}
	}
}

// TestTorchBlockLight places a torch in a sealed room and checks that its block
// light fills the room, attenuates, is stopped by solid walls, and vanishes when
// the torch is removed.
func TestTorchBlockLight(t *testing.T) {
	w := NewWorld()
	c := NewChunk(Vec3{0, 0, 0})
	// Solid stone at [10,14]^3 with a hollow 3x3x3 air interior [11,13]^3.
	for x := 10; x <= 14; x++ {
		for y := 10; y <= 14; y++ {
			for z := 10; z <= 14; z++ {
				interior := x >= 11 && x <= 13 && y >= 11 && y <= 13 && z >= 11 && z <= 13
				if !interior {
					c.add(Vec3{x, y, z}, 4)
				}
			}
		}
	}
	w.storeChunk(c.id, c)
	w.seedChunkBatch([]*Chunk{c})

	corner := Vec3{11, 11, 11}
	if got := w.light(corner); got != 0 {
		t.Fatalf("sealed interior skylight = %d, want 0", got)
	}
	if got := w.getBlockLight(corner); got != 0 {
		t.Fatalf("interior block light before torch = %d, want 0", got)
	}

	// Place a torch in a corner of the interior.
	c.add(corner, blockTorch)
	w.updateBlockLight(corner, 0, blockTorch)

	if got := w.getBlockLight(corner); got != torchLight {
		t.Errorf("torch cell block light = %d, want %d", got, torchLight)
	}
	if got := w.getBlockLight(Vec3{12, 11, 11}); got != torchLight-1 {
		t.Errorf("one step from torch = %d, want %d", got, torchLight-1)
	}
	if got := w.getBlockLight(Vec3{13, 13, 13}); got != torchLight-6 {
		t.Errorf("far interior corner = %d, want %d", got, torchLight-6)
	}
	if got := w.getBlockLight(Vec3{10, 11, 11}); got != 0 {
		t.Errorf("solid shell = %d, want 0 (opaque blocks light)", got)
	}

	// Remove the torch: block light goes away everywhere.
	c.del(corner)
	w.updateBlockLight(corner, blockTorch, 0)
	if got := w.getBlockLight(corner); got != 0 {
		t.Errorf("after removal, torch cell = %d, want 0", got)
	}
	if got := w.getBlockLight(Vec3{13, 13, 13}); got != 0 {
		t.Errorf("after removal, far corner = %d, want 0", got)
	}
}

// TestTorchSeededOnLoad checks that a torch present when a chunk is first lit
// (e.g. a persisted torch) gets its block light seeded.
func TestTorchSeededOnLoad(t *testing.T) {
	w := NewWorld()
	c := NewChunk(Vec3{0, 0, 0})
	torch := Vec3{5, 5, 5}
	c.add(torch, blockTorch)
	w.storeChunk(c.id, c)
	w.seedChunkBatch([]*Chunk{c})

	if got := w.getBlockLight(torch); got != torchLight {
		t.Errorf("seeded torch = %d, want %d", got, torchLight)
	}
	if got := w.getBlockLight(Vec3{6, 5, 5}); got != torchLight-1 {
		t.Errorf("one step from seeded torch = %d, want %d", got, torchLight-1)
	}
}

// TestParallelSeedBlockLightMatchesReference checks that the parallel two-phase
// seed produces the same block-light field as the authoritative global seed,
// with a torch whose light crosses a chunk border.
func TestParallelSeedBlockLightMatchesReference(t *testing.T) {
	w := NewWorld()
	var chunks []*Chunk
	for cx := 0; cx <= 1; cx++ {
		c := NewChunk(Vec3{cx, 0, 0})
		w.storeChunk(c.id, c)
		chunks = append(chunks, c)
	}
	// A torch near the x=31|32 chunk border, in open air, so its light crosses
	// into the next chunk.
	chunks[0].add(Vec3{30, 40, 5}, blockTorch)

	w.seedRegionReference(chunks)
	ref := map[Vec3]uint8{}
	for _, c := range chunks {
		bx := c.id.X * ChunkWidth
		for lx := 0; lx < ChunkWidth; lx++ {
			for lz := 0; lz < ChunkWidth; lz++ {
				for y := 30; y <= 50; y++ {
					p := Vec3{bx + lx, y, lz}
					ref[p] = c.getBlockLight(p)
				}
			}
		}
	}
	if ref[Vec3{34, 40, 5}] == 0 {
		t.Fatalf("test setup: torch light did not cross the chunk border")
	}

	for _, c := range chunks {
		c.blockLight = nil
	}
	w.seedChunkBatch(chunks)

	mismatches := 0
	for _, c := range chunks {
		bx := c.id.X * ChunkWidth
		for lx := 0; lx < ChunkWidth; lx++ {
			for lz := 0; lz < ChunkWidth; lz++ {
				for y := 30; y <= 50; y++ {
					p := Vec3{bx + lx, y, lz}
					if got := c.getBlockLight(p); got != ref[p] {
						if mismatches < 6 {
							t.Errorf("block-light mismatch at %v: parallel=%d reference=%d", p, got, ref[p])
						}
						mismatches++
					}
				}
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d block-light cells differ between parallel and reference", mismatches)
	}
}
