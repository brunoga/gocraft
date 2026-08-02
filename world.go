package main

import (
	"log"
	"sync"

	"github.com/go-gl/mathgl/mgl32"
	lru "github.com/hashicorp/golang-lru"
)

type World struct {
	mutex  sync.Mutex
	chunks *lru.Cache // map[Vec3]*Chunk

	// lightMu guards all per-voxel light state (Chunk.light) and the light
	// propagation algorithms. Writers (seeding, edits) take it exclusively; mesh
	// workers take it for reading (RLock) so many chunks can mesh at once.
	// lightDirty collects chunks whose light changed and need re-meshing.
	lightMu    sync.RWMutex
	lightDirty map[Vec3]bool
	// lcPtr is a one-entry cache of the chunk most recently touched by the light
	// grid accessors. Light propagation is overwhelmingly within a single chunk,
	// so this skips the LRU lookup (and its lock) for the common case. Guarded by
	// lightMu; reset at the start of each light operation.
	lcPtr *Chunk
}

func NewWorld() *World {
	m := (*renderRadius) * (*renderRadius) * 4
	chunks, _ := lru.New(m)
	return &World{
		chunks:     chunks,
		lightDirty: make(map[Vec3]bool),
	}
}

// --- lightGrid implementation (see light.go). All methods are lock-free; the
// high-level operations that call them hold lightMu. ---

// lightChunk resolves a chunk for the light accessors via the one-entry cache,
// falling back to an LRU peek. Callers hold lightMu.
func (w *World) lightChunk(cid Vec3) *Chunk {
	if w.lcPtr != nil && w.lcPtr.id == cid {
		return w.lcPtr
	}
	c := w.peekChunk(cid)
	if c != nil {
		w.lcPtr = c
	}
	return c
}

// resetLightCache clears the one-entry chunk cache. Call at the start of each
// light operation so a stale (e.g. evicted) chunk pointer isn't reused.
func (w *World) resetLightCache() { w.lcPtr = nil }

func (w *World) blocksLight(p Vec3) bool {
	if p.Y < 0 {
		return true
	}
	if p.Y >= WorldHeight {
		return false
	}
	c := w.lightChunk(p.Chunkid())
	if c == nil {
		return false
	}
	return aoOccludes(c.Block(p))
}

func (w *World) loaded(p Vec3) bool {
	if p.Y < 0 || p.Y >= WorldHeight {
		return false
	}
	return w.lightChunk(p.Chunkid()) != nil
}

func (w *World) light(p Vec3) uint8 {
	if p.Y >= WorldHeight {
		return MaxLight
	}
	if p.Y < 0 {
		return 0
	}
	c := w.lightChunk(p.Chunkid())
	if c == nil {
		// Unloaded neighbour: assume lit so border faces don't show dark seams.
		return MaxLight
	}
	return c.getLight(p)
}

func (w *World) setLight(p Vec3, v uint8) {
	if p.Y < 0 || p.Y >= WorldHeight {
		return
	}
	c := w.lightChunk(p.Chunkid())
	if c == nil {
		return
	}
	c.setLight(p, v)
}

func (w *World) peekChunk(cid Vec3) *Chunk {
	v, ok := w.chunks.Peek(cid)
	if !ok {
		return nil
	}
	return v.(*Chunk)
}

// getBlockLight / setBlockLight are the block-light (torch) counterparts of
// light / setLight. Unlike skylight there is no open-sky fallback: an unloaded
// or out-of-range cell simply carries no block light.
func (w *World) getBlockLight(p Vec3) uint8 {
	if p.Y < 0 || p.Y >= WorldHeight {
		return 0
	}
	c := w.lightChunk(p.Chunkid())
	if c == nil {
		return 0
	}
	return c.getBlockLight(p)
}

func (w *World) setBlockLight(p Vec3, v uint8) {
	if p.Y < 0 || p.Y >= WorldHeight {
		return
	}
	c := w.lightChunk(p.Chunkid())
	if c == nil {
		return
	}
	c.setBlockLight(p, v)
}

// blockGrid views the world's block-light field as a lightGrid, so the same
// propagation code (light.go) drives both skylight and block light. It shares
// blocksLight/loaded with skylight but reads/writes the block-light field.
type blockGrid struct{ w *World }

func (g blockGrid) blocksLight(p Vec3) bool  { return g.w.blocksLight(p) }
func (g blockGrid) loaded(p Vec3) bool       { return g.w.loaded(p) }
func (g blockGrid) light(p Vec3) uint8       { return g.w.getBlockLight(p) }
func (g blockGrid) setLight(p Vec3, v uint8) { g.w.setBlockLight(p, v) }

// lightTop is the highest cell seeding fills: the top of the world. Cells above
// are handled by the WorldHeight guard in light().
func lightTop(*Chunk) int { return WorldHeight - 1 }

// markLightDirty records the chunks containing the touched cells so the render
// loop re-meshes them. Callers must hold lightMu.
func (w *World) markLightDirty(touched map[Vec3]bool) {
	for p := range touched {
		w.lightDirty[p.Chunkid()] = true
	}
}

// DrainLightDirty returns and clears the set of chunks whose light has changed
// since the last call.
func (w *World) DrainLightDirty() []Vec3 {
	w.lightMu.Lock()
	defer w.lightMu.Unlock()
	if len(w.lightDirty) == 0 {
		return nil
	}
	ids := make([]Vec3, 0, len(w.lightDirty))
	for id := range w.lightDirty {
		ids = append(ids, id)
		delete(w.lightDirty, id)
	}
	return ids
}

func (w *World) loadChunk(id Vec3) (*Chunk, bool) {
	chunk, ok := w.chunks.Get(id)
	if !ok {
		return nil, false
	}
	return chunk.(*Chunk), true
}

func (w *World) storeChunk(id Vec3, chunk *Chunk) {
	w.chunks.Add(id, chunk)
}

func (w *World) Collide(pos mgl32.Vec3) (mgl32.Vec3, bool) {
	x, y, z := pos.X(), pos.Y(), pos.Z()
	nx, ny, nz := round(pos.X()), round(pos.Y()), round(pos.Z())
	const pad = 0.25

	head := Vec3{int(nx), int(ny), int(nz)}
	foot := head.Down()

	stop := false
	for _, b := range []Vec3{foot, head} {
		if IsObstacle(w.Block(b.Left())) && x < nx && nx-x > pad {
			x = nx - pad
		}
		if IsObstacle(w.Block(b.Right())) && x > nx && x-nx > pad {
			x = nx + pad
		}
		if IsObstacle(w.Block(b.Down())) && y < ny && ny-y > pad {
			y = ny - pad
			stop = true
		}
		if IsObstacle(w.Block(b.Up())) && y > ny && y-ny > pad {
			y = ny + pad
			stop = true
		}
		if IsObstacle(w.Block(b.Back())) && z < nz && nz-z > pad {
			z = nz - pad
		}
		if IsObstacle(w.Block(b.Front())) && z > nz && z-nz > pad {
			z = nz + pad
		}
	}
	return mgl32.Vec3{x, y, z}, stop
}

// maxCollideStep is the largest movement (in blocks) resolved in a single
// Collide call. Movement longer than this is subdivided by CollideStepped.
const maxCollideStep = 0.2

// CollideStepped resolves movement from `from` to `to` by walking the path in
// steps no larger than maxCollideStep and resolving collisions at each step.
// Collide only pushes out of blocks adjacent to a single position, so a large
// per-frame move (e.g. while flying at 5x speed) could otherwise skip straight
// through a thin wall. Stepping keeps every intermediate position collision-free
// and lets the player slide along surfaces. It returns the final position and
// whether vertical movement was stopped (used to zero out fall velocity).
func (w *World) CollideStepped(from, to mgl32.Vec3) (mgl32.Vec3, bool) {
	delta := to.Sub(from)
	dist := delta.Len()
	if dist < maxCollideStep {
		return w.Collide(to)
	}

	steps := int(dist/maxCollideStep) + 1
	step := delta.Mul(1 / float32(steps))
	pos := from
	stop := false
	for i := 0; i < steps; i++ {
		var s bool
		pos, s = w.Collide(pos.Add(step))
		if s {
			stop = true
		}
	}
	return pos, stop
}

func (w *World) HitTest(pos mgl32.Vec3, vec mgl32.Vec3) (*Vec3, *Vec3) {
	var (
		maxLen = float32(8.0)
		step   = float32(0.125)

		block, prev Vec3
		pprev       *Vec3
	)

	for len := float32(0); len < maxLen; len += step {
		block = NearBlock(pos.Add(vec.Mul(len)))
		if prev != block && w.HasBlock(block) {
			return &block, pprev
		}
		prev = block
		pprev = &prev
	}
	return nil, nil
}

func (w *World) Block(id Vec3) int {
	chunk := w.BlockChunk(id)
	if chunk == nil {
		return -1
	}
	return chunk.Block(id)
}

func (w *World) BlockChunk(block Vec3) *Chunk {
	cid := block.Chunkid()
	chunk, ok := w.loadChunk(cid)
	if !ok {
		return nil
	}
	return chunk
}

// setBlockTransient changes a block and updates lighting WITHOUT persisting to
// the store, for transient simulation changes (fire) that shouldn't be saved.
func (w *World) setBlockTransient(pos Vec3, old, tp int) {
	chunk := w.BlockChunk(pos)
	if chunk == nil {
		return
	}
	if tp != 0 {
		chunk.add(pos, tp)
	} else {
		chunk.del(pos)
	}
	w.updateBlockLight(pos, old, tp)
}

func (w *World) UpdateBlock(id Vec3, tp int) {
	chunk := w.BlockChunk(id)
	if chunk != nil {
		old := chunk.Block(id)
		if tp != 0 {
			chunk.add(id, tp)
		} else {
			chunk.del(id)
		}
		w.updateBlockLight(id, old, tp)
	}
	store.UpdateBlock(id, tp)
}

// updateBlockLight incrementally repairs lighting after the block at id changed
// from oldTp to newTp. Only a change in opacity affects light. The block change
// must already be applied so blocksLight(id) reflects newTp.
func (w *World) updateBlockLight(id Vec3, oldTp, newTp int) {
	if id.Y < 0 || id.Y >= WorldHeight {
		return
	}
	wasOpaque := aoOccludes(oldTp)
	nowOpaque := aoOccludes(newTp)
	opacityChanged := wasOpaque != nowOpaque
	emissionChanged := blockEmission(oldTp) != blockEmission(newTp)
	if !opacityChanged && !emissionChanged {
		return // neither skylight nor block light is affected
	}
	w.lightMu.Lock()
	defer w.lightMu.Unlock()
	w.resetLightCache()
	touched := make(map[Vec3]bool)
	// Skylight only cares about opacity.
	if opacityChanged {
		if nowOpaque {
			lightPlaceBlock(w, id, touched)
		} else {
			lightRemoveBlock(w, id, WorldHeight-1, touched)
		}
	}
	// Block light cares about opacity and emission.
	w.updateBlockLightAt(id, oldTp, newTp, touched)
	w.markLightDirty(touched)
}

func IsPlant(tp int) bool {
	if tp >= 17 && tp <= 31 {
		return true
	}
	return false
}

func IsTransparent(tp int) bool {
	if IsPlant(tp) || isTorch(tp) || isFire(tp) {
		return true
	}
	switch tp {
	case -1, 0, 10, 15:
		return true
	default:
		return false
	}
}

// aoOccludes reports whether a block type casts ambient occlusion onto its
// neighbours. Opaque solid blocks occlude; air, unloaded chunks (-1), plants,
// glass and leaves (all transparent) do not.
func aoOccludes(tp int) bool {
	return !IsTransparent(tp)
}

func IsObstacle(tp int) bool {
	if IsPlant(tp) || isTorch(tp) || isFire(tp) {
		return false
	}
	switch tp {
	case -1:
		return true
	case 0:
		return false
	default:
		return true
	}
}

func (w *World) HasBlock(id Vec3) bool {
	tp := w.Block(id)
	return tp != -1 && tp != 0
}

// genChunk generates and stores a chunk WITHOUT seeding its light. It returns
// nil if the chunk already exists or generation fails.
func (w *World) genChunk(id Vec3) *Chunk {
	if _, ok := w.loadChunk(id); ok {
		return nil
	}
	chunk := NewChunk(id)
	blocks := makeChunkMap(id)
	for block, tp := range blocks {
		chunk.add(block, tp)
	}
	err := store.RangeBlocks(id, func(bid Vec3, w int) {
		if w == 0 {
			chunk.del(bid)
			return
		}
		chunk.add(bid, w)
	})
	if err != nil {
		log.Printf("fetch chunk(%v) from db error:%s", id, err)
		return nil
	}
	ClientFetchChunk(id, func(bid Vec3, w int) {
		if w == 0 {
			chunk.del(bid)
			return
		}
		chunk.add(bid, w)
		store.UpdateBlock(bid, w)
	})
	w.storeChunk(id, chunk)
	return chunk
}

func (w *World) Chunk(id Vec3) *Chunk {
	if c, ok := w.loadChunk(id); ok {
		return c
	}
	c := w.genChunk(id)
	if c == nil {
		// Already loaded by a concurrent caller, or generation failed.
		if existing, ok := w.loadChunk(id); ok {
			return existing
		}
		return nil
	}
	w.seedChunkBatch([]*Chunk{c})
	return c
}

func (w *World) Chunks(ids []Vec3) []*Chunk {
	// Generate all chunks in parallel without seeding light...
	type result struct {
		c     *Chunk
		fresh bool
	}
	ch := make(chan result)
	for _, id := range ids {
		id := id
		go func() {
			if existing, ok := w.loadChunk(id); ok {
				ch <- result{existing, false}
				return
			}
			c := w.genChunk(id)
			if c == nil {
				existing, _ := w.loadChunk(id)
				ch <- result{existing, false}
				return
			}
			ch <- result{c, true}
		}()
	}
	var chunks, fresh []*Chunk
	for range ids {
		r := <-ch
		if r.c != nil {
			chunks = append(chunks, r.c)
		}
		if r.fresh {
			fresh = append(fresh, r.c)
		}
	}
	// ...then seed the freshly generated ones as one batch: phase-1 parallel,
	// phase-2 border stitch.
	w.seedChunkBatch(fresh)
	return chunks
}

// makeChunkMap generates one chunk's blocks by sampling the procedural
// generator in worldgen.go: layered terrain (surface/dirt/stone) carved by cave
// tunnels, flowers on open grass, forests whose canopies span chunk seams, and a
// high cloud layer. Everything it writes stays within this chunk's column so
// chunk.add never sees a foreign block.
func makeChunkMap(cid Vec3) map[Vec3]int {
	m := make(map[Vec3]int)
	x0, z0 := cid.X*ChunkWidth, cid.Z*ChunkWidth

	for dx := 0; dx < ChunkWidth; dx++ {
		for dz := 0; dz < ChunkWidth; dz++ {
			x, z := x0+dx, z0+dz
			surfaceY := terrainHeight(x, z)

			// Solid column from the floor up to the surface, hollowed by caves.
			for y := 0; y <= surfaceY; y++ {
				if caveCarved(x, y, z) && y < surfaceY {
					continue // leave air; keep the very top block as a crust
				}
				m[Vec3{x, y, z}] = columnBlock(x, z, y, surfaceY)
			}

			// Flowers and tufts of grass on open ground.
			if surf, _ := surfaceBlocks(x, z, surfaceY); surf == genGrass {
				if noise2(-float32(x)*0.1, float32(z)*0.1, 4, 0.8, 2) > 0.62 {
					m[Vec3{x, surfaceY + 1, z}] = 17
				} else if noise2(float32(x)*0.05, float32(-z)*0.05, 4, 0.8, 2) > 0.72 {
					m[Vec3{x, surfaceY + 1, z}] = 18 + int(noise2(float32(x)*0.1, float32(z)*0.1, 4, 0.8, 2)*7)
				}
			}

			// Clouds drifting well above the peaks.
			for y := genCloudLow; y < genCloudHigh; y++ {
				if noise3(float32(x)*0.01, float32(y)*0.1, float32(z)*0.01, 8, 0.5, 2) > 0.70 {
					m[Vec3{x, y, z}] = genCloud
				}
			}
		}
	}

	// Trees: scan every tree cell that could reach into this chunk (a canopy
	// extends up to treeCell blocks past a seam) and stamp the parts that land
	// here. Neighbouring chunks stamp the rest, so trees cross seams cleanly.
	cxLo := floorDiv(x0-treeCell, treeCell)
	cxHi := floorDiv(x0+ChunkWidth+treeCell, treeCell)
	czLo := floorDiv(z0-treeCell, treeCell)
	czHi := floorDiv(z0+ChunkWidth+treeCell, treeCell)
	for cx := cxLo; cx <= cxHi; cx++ {
		for cz := czLo; cz <= czHi; cz++ {
			if t, ok := treeInCell(cx, cz); ok {
				addTree(m, t, x0, z0)
			}
		}
	}
	return m
}
