package main

// Voxel light propagation. Light levels run 0 (dark) .. MaxLight (fully lit).
// Today the only source is skylight (cells with an unobstructed vertical path
// to the top of the world), but the propagation algorithms are source-agnostic:
// adding block-light sources later just means seeding the increase queue with
// emissive cells and taking max(sky, block) when shading.
//
// The algorithms operate on the lightGrid interface so they can run against the
// real world in the game and against a simple in-memory grid in tests.

const MaxLight = 15

// WorldHeight bounds the vertical range that carries per-voxel light. Terrain
// (up to snow-capped mountain peaks), trees and clouds all sit well below this;
// cells at or above it are treated as open sky (fully lit). It is tall enough to
// give mountain ridges dramatic relief and still float clouds above them.
const WorldHeight = 256

// lightGrid is the storage the propagation algorithms read and write.
type lightGrid interface {
	// blocksLight reports whether the block at p stops light (opaque solid).
	blocksLight(p Vec3) bool
	// loaded reports whether p is a cell we may light. Propagation stops at
	// unloaded cells (outside the world bounds or in an unloaded chunk).
	loaded(p Vec3) bool
	light(p Vec3) uint8
	setLight(p Vec3, v uint8)
}

var lightDirs = [6]Vec3{
	{1, 0, 0}, {-1, 0, 0},
	{0, 1, 0}, {0, -1, 0},
	{0, 0, 1}, {0, 0, -1},
}

func addVec(a, b Vec3) Vec3 { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

// lightNode pairs a cell with a light level, used as a work item for removal.
type lightNode struct {
	p   Vec3
	lvl uint8
}

// propagateIncrease floods light outward from the queued cells, which must
// already have their light values set. Every cell whose light it raises is
// recorded in touched (may be nil). The queue grows in place as a BFS frontier.
func propagateIncrease(g lightGrid, queue []Vec3, touched map[Vec3]bool) {
	for i := 0; i < len(queue); i++ {
		c := queue[i]
		l := g.light(c)
		if l <= 1 {
			continue
		}
		for _, d := range lightDirs {
			n := addVec(c, d)
			if !g.loaded(n) || g.blocksLight(n) {
				continue
			}
			if g.light(n) < l-1 {
				g.setLight(n, l-1)
				if touched != nil {
					touched[n] = true
				}
				queue = append(queue, n)
			}
		}
	}
}

// propagateDecrease removes light starting from the given nodes (cells whose
// light is being taken away, paired with the level they had). It zeroes cells
// that were lit by the removed light and returns the cells that border still
// brighter light, which must be re-flooded with propagateIncrease to fill the
// darkened region back in. Every cell it zeroes is recorded in touched.
func propagateDecrease(g lightGrid, removals []lightNode, touched map[Vec3]bool) []Vec3 {
	var relight []Vec3
	for i := 0; i < len(removals); i++ {
		c := removals[i].p
		l := removals[i].lvl
		for _, d := range lightDirs {
			n := addVec(c, d)
			if !g.loaded(n) {
				continue
			}
			ln := g.light(n)
			if ln == 0 {
				continue
			}
			if ln < l {
				// n was lit (directly or via a chain) by the removed light.
				g.setLight(n, 0)
				if touched != nil {
					touched[n] = true
				}
				removals = append(removals, lightNode{n, ln})
			} else {
				// n is at least as bright: an independent source that will
				// re-light part of the hole.
				relight = append(relight, n)
			}
		}
	}
	return relight
}

// lightPlaceBlock updates lighting after an opaque block is placed at p. The
// grid must already report blocksLight(p)==true. Every changed cell is recorded
// in touched.
func lightPlaceBlock(g lightGrid, p Vec3, touched map[Vec3]bool) {
	var removals []lightNode
	// The block's own cell no longer holds light.
	if l := g.light(p); l > 0 {
		g.setLight(p, 0)
		if touched != nil {
			touched[p] = true
		}
		removals = append(removals, lightNode{p, l})
	}
	// Cells directly below lose direct skylight: they were at MaxLight only
	// because the column above was open, and only a vertical sky path yields
	// MaxLight. Remove that skylight; horizontal neighbours re-light them to a
	// lower level in the increase pass below.
	for y := p.Y - 1; y >= 0; y-- {
		c := Vec3{p.X, y, p.Z}
		if !g.loaded(c) || g.blocksLight(c) {
			break
		}
		if g.light(c) == MaxLight {
			g.setLight(c, 0)
			if touched != nil {
				touched[c] = true
			}
			removals = append(removals, lightNode{c, MaxLight})
		}
	}
	relight := propagateDecrease(g, removals, touched)
	propagateIncrease(g, relight, touched)
}

// lightRemoveBlock updates lighting after the opaque block at p is removed (p is
// now air). The grid must already report blocksLight(p)==false. Every changed
// cell is recorded in touched.
func lightRemoveBlock(g lightGrid, p Vec3, topY int, touched map[Vec3]bool) {
	// Re-seed skylight down this column: p, and cells below it down to the next
	// block, may now have a clear vertical path to the sky.
	queue := seedSkyColumn(g, p.X, p.Z, topY, touched)
	// Also pull light in from lit neighbours of the newly opened cell.
	for _, d := range lightDirs {
		n := addVec(p, d)
		if g.loaded(n) && g.light(n) > 0 {
			queue = append(queue, n)
		}
	}
	propagateIncrease(g, queue, touched)
}

// seedSkyColumn sets full skylight on every cell of column (x,z) that has an
// unobstructed vertical path down from topY, stopping at the first block that
// blocks light (everything below it is shadowed from direct sky). It returns
// the seeded cells so they can be used as increase sources, and records raised
// cells in touched.
func seedSkyColumn(g lightGrid, x, z, topY int, touched map[Vec3]bool) []Vec3 {
	var seeds []Vec3
	for y := topY; y >= 0; y-- {
		p := Vec3{x, y, z}
		if !g.loaded(p) {
			continue
		}
		if g.blocksLight(p) {
			break
		}
		if g.light(p) < MaxLight {
			g.setLight(p, MaxLight)
			if touched != nil {
				touched[p] = true
			}
		}
		seeds = append(seeds, p)
	}
	return seeds
}
