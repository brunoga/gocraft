package main

import "sync"

// Parallel two-phase skylight seeding for a batch of freshly loaded chunks:
//
//	Phase 1 (parallel): seed each chunk's own skylight in isolation, clamped to
//	  the chunk so workers touch disjoint light arrays and need no locking.
//	Phase 2 (serial): stitch light across chunk borders, where a cell on one
//	  side of a seam is brighter than the cell across it.
//
// The result is identical to seeding every chunk's sky and running one global
// propagation (see TestParallelSeedMatchesReference), but most of the work (the
// per-chunk propagation) runs across all cores.

// chunkGrid is a lightGrid restricted to a single chunk. Propagation through it
// stops at the chunk borders (loaded() is false outside), so seeding one chunk
// never writes another chunk's cells.
type chunkGrid struct {
	c            *Chunk
	baseX, baseZ int
	top          int
}

func newChunkGrid(c *Chunk) *chunkGrid {
	return &chunkGrid{
		c:     c,
		baseX: c.id.X * ChunkWidth,
		baseZ: c.id.Z * ChunkWidth,
		top:   lightTop(c),
	}
}

func (g *chunkGrid) inBounds(p Vec3) bool {
	lx, lz := p.X-g.baseX, p.Z-g.baseZ
	return lx >= 0 && lx < ChunkWidth && lz >= 0 && lz < ChunkWidth && p.Y >= 0 && p.Y <= g.top
}

func (g *chunkGrid) loaded(p Vec3) bool { return g.inBounds(p) }

// blocksLight is only ever called for in-bounds cells (propagation checks
// loaded() first), so c.Block is safe.
func (g *chunkGrid) blocksLight(p Vec3) bool  { return aoOccludes(g.c.Block(p)) }
func (g *chunkGrid) light(p Vec3) uint8       { return g.c.getLight(p) }
func (g *chunkGrid) setLight(p Vec3, v uint8) { g.c.setLight(p, v) }

// seedChunkLocal seeds a chunk's skylight in isolation. It touches only c's own
// arrays, so it is safe to run concurrently for different chunks.
func seedChunkLocal(c *Chunk) {
	g := newChunkGrid(c)
	var q []Vec3
	for dx := 0; dx < ChunkWidth; dx++ {
		for dz := 0; dz < ChunkWidth; dz++ {
			q = append(q, seedSkyColumn(g, g.baseX+dx, g.baseZ+dz, g.top, nil)...)
		}
	}
	propagateIncrease(g, q, nil)
}

// seedChunkBatch seeds the skylight of the given freshly loaded chunks: phase 1
// in parallel, then a serial border stitch. Callers must NOT hold lightMu; this
// takes it (blocking meshing/edits) for the whole batch, but the phase-1 workers
// run lock-free on disjoint chunks while it is held.
func (w *World) seedChunkBatch(chunks []*Chunk) {
	if len(chunks) == 0 {
		return
	}
	w.lightMu.Lock()
	defer w.lightMu.Unlock()

	// Phase 1: seed each chunk's own skylight in parallel.
	var wg sync.WaitGroup
	for _, c := range chunks {
		wg.Add(1)
		go func(c *Chunk) {
			defer wg.Done()
			seedChunkLocal(c)
		}(c)
	}
	wg.Wait()

	// Phase 2: stitch light across chunk borders on the world grid.
	w.resetLightCache()
	touched := make(map[Vec3]bool)
	propagateIncrease(w, w.batchBorderSources(chunks), touched)

	// Every newly seeded chunk needs a first mesh; the stitch may also have
	// changed already-loaded neighbours.
	for _, c := range chunks {
		w.lightDirty[c.id] = true
	}
	w.markLightDirty(touched)
}

// batchBorderSources returns the cells that should propagate light across a
// chunk seam after phase 1: for each border cell pair (a inside a batch chunk,
// b across the seam), whichever side is brighter by more than one level. Cells
// where both sides already agree contribute nothing, so for continuous surface
// terrain this is nearly empty and only caves/tunnels crossing a seam matter.
func (w *World) batchBorderSources(chunks []*Chunk) []Vec3 {
	var q []Vec3
	consider := func(a, b Vec3) {
		if !w.loaded(b) {
			return
		}
		la, lb := w.light(a), w.light(b)
		if la > lb+1 {
			q = append(q, a)
		} else if lb > la+1 {
			q = append(q, b)
		}
	}
	for _, c := range chunks {
		bx, bz := c.id.X*ChunkWidth, c.id.Z*ChunkWidth
		top := lightTop(c)
		for y := 0; y <= top; y++ {
			for k := 0; k < ChunkWidth; k++ {
				consider(Vec3{bx, y, bz + k}, Vec3{bx - 1, y, bz + k})
				consider(Vec3{bx + ChunkWidth - 1, y, bz + k}, Vec3{bx + ChunkWidth, y, bz + k})
				consider(Vec3{bx + k, y, bz}, Vec3{bx + k, y, bz - 1})
				consider(Vec3{bx + k, y, bz + ChunkWidth - 1}, Vec3{bx + k, y, bz + ChunkWidth})
			}
		}
	}
	return q
}
