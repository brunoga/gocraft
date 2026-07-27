package main

import "testing"

// buildTestChunk creates a synthetic chunk: a solid ground slab (z in 40..50,
// y in 0..20) with a 1-wide tunnel bored along X at y=10,z=45, lit only by a
// vertical shaft to the sky at x=30,z=45. The shaft sits near the x=31|32 chunk
// border, so tunnel light must cross chunk seams -- exercising the phase-2
// stitch.
func buildTestChunk(cid Vec3) *Chunk {
	const stone = 4
	c := NewChunk(cid)
	bx, bz := cid.X*ChunkWidth, cid.Z*ChunkWidth
	for lx := 0; lx < ChunkWidth; lx++ {
		x := bx + lx
		for lz := 0; lz < ChunkWidth; lz++ {
			z := bz + lz
			if z < 40 || z > 50 {
				continue
			}
			for y := 0; y <= 20; y++ {
				air := (y == 10 && z == 45) || // tunnel
					(x == 30 && z == 45 && y >= 10) // shaft to sky
				if !air {
					c.add(Vec3{x, y, z}, stone)
				}
			}
		}
	}
	return c
}

// seedRegionReference lights the given stored chunks the authoritative way: seed
// every chunk's sky columns, then one global propagation. Order-independent, so
// it is the ground truth the parallel batch must match.
func (w *World) seedRegionReference(chunks []*Chunk) {
	for _, c := range chunks {
		c.light = nil
		c.blockLight = nil
	}
	w.resetLightCache()

	// Skylight: seed all sky columns, then one global propagation.
	var q []Vec3
	for _, c := range chunks {
		bx, bz := c.id.X*ChunkWidth, c.id.Z*ChunkWidth
		top := lightTop(c)
		for dx := 0; dx < ChunkWidth; dx++ {
			for dz := 0; dz < ChunkWidth; dz++ {
				q = append(q, seedSkyColumn(w, bx+dx, bz+dz, top, nil)...)
			}
		}
	}
	propagateIncrease(w, q, nil)

	// Block light: seed all emitters, then one global propagation.
	var bq []Vec3
	bg := blockGrid{w}
	for _, c := range chunks {
		c.RangeBlocks(func(id Vec3, tp int) {
			if e := blockEmission(tp); e > 0 {
				bg.setLight(id, e)
				bq = append(bq, id)
			}
		})
	}
	propagateIncrease(bg, bq, nil)
}

func TestParallelSeedMatchesReference(t *testing.T) {
	w := NewWorld()
	var chunks []*Chunk
	for cx := 0; cx <= 2; cx++ {
		for cz := 0; cz <= 2; cz++ {
			c := buildTestChunk(Vec3{cx, 0, cz})
			w.storeChunk(c.id, c)
			chunks = append(chunks, c)
		}
	}

	// Ground truth.
	w.seedRegionReference(chunks)
	ref := map[Vec3]uint8{}
	for _, c := range chunks {
		bx, bz := c.id.X*ChunkWidth, c.id.Z*ChunkWidth
		for lx := 0; lx < ChunkWidth; lx++ {
			for lz := 0; lz < ChunkWidth; lz++ {
				for y := 0; y <= 25; y++ {
					p := Vec3{bx + lx, y, bz + lz}
					ref[p] = c.getLight(p)
				}
			}
		}
	}

	// Sanity: the shaft lights the tunnel across the chunk seam (into chunk x=1),
	// so the test actually exercises cross-chunk stitching.
	if got := ref[Vec3{40, 10, 45}]; got == 0 {
		t.Fatalf("test setup: tunnel cell across the seam is dark (%d); no cross-chunk light to check", got)
	}

	// Parallel two-phase seed from scratch.
	for _, c := range chunks {
		c.light = nil
	}
	w.seedChunkBatch(chunks)

	// Every cell must match the reference exactly.
	mismatches := 0
	for _, c := range chunks {
		bx, bz := c.id.X*ChunkWidth, c.id.Z*ChunkWidth
		for lx := 0; lx < ChunkWidth; lx++ {
			for lz := 0; lz < ChunkWidth; lz++ {
				for y := 0; y <= 25; y++ {
					p := Vec3{bx + lx, y, bz + lz}
					if got := c.getLight(p); got != ref[p] {
						if mismatches < 8 {
							t.Errorf("light mismatch at %v: parallel=%d reference=%d", p, got, ref[p])
						}
						mismatches++
					}
				}
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d cells differ between parallel batch and reference", mismatches)
	}
}

// The parallel seed must be deterministic despite running across goroutines.
func TestParallelSeedIsDeterministic(t *testing.T) {
	seed := func() map[Vec3]uint8 {
		w := NewWorld()
		var chunks []*Chunk
		for cx := 0; cx <= 2; cx++ {
			for cz := 0; cz <= 2; cz++ {
				c := buildTestChunk(Vec3{cx, 0, cz})
				w.storeChunk(c.id, c)
				chunks = append(chunks, c)
			}
		}
		w.seedChunkBatch(chunks)
		out := map[Vec3]uint8{}
		for _, c := range chunks {
			bx, bz := c.id.X*ChunkWidth, c.id.Z*ChunkWidth
			for lx := 0; lx < ChunkWidth; lx++ {
				for lz := 0; lz < ChunkWidth; lz++ {
					for y := 0; y <= 25; y++ {
						p := Vec3{bx + lx, y, bz + lz}
						out[p] = c.getLight(p)
					}
				}
			}
		}
		return out
	}
	a, b := seed(), seed()
	if len(a) != len(b) {
		t.Fatalf("snapshot sizes differ: %d vs %d", len(a), len(b))
	}
	for p, va := range a {
		if b[p] != va {
			t.Fatalf("nondeterministic light at %v: %d vs %d", p, va, b[p])
		}
	}
}
