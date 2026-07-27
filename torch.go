package main

// blockTorch is an emissive, non-solid block that lights up its surroundings.
const blockTorch = 65

// torchLight is the block-light level a torch emits.
const torchLight = 14

// blockEmission returns the block-light level a block type emits (0 if none).
func blockEmission(tp int) uint8 {
	switch tp {
	case blockTorch:
		return torchLight
	case blockFire:
		return fireLight
	}
	return 0
}

func isTorch(tp int) bool { return tp == blockTorch }

// updateBlockLightAt repairs the block-light field after the block at id changed
// from oldTp to newTp (the change is already applied to the chunk). Callers hold
// lightMu. Cases:
//   - a new emitter becomes a light source;
//   - a removed emitter, or a cell that became opaque, loses its light;
//   - a removed opaque block lets neighbouring block light flow in.
func (w *World) updateBlockLightAt(id Vec3, oldTp, newTp int, touched map[Vec3]bool) {
	g := blockGrid{w}
	oldEmit := blockEmission(oldTp)
	newEmit := blockEmission(newTp)
	wasOpaque := aoOccludes(oldTp)
	nowOpaque := aoOccludes(newTp)

	// Remove light if an emitter went away or the cell now blocks light.
	if oldEmit > 0 || (nowOpaque && !wasOpaque) {
		if old := g.light(id); old > 0 {
			g.setLight(id, 0)
			relight := propagateDecrease(g, []lightNode{{id, old}}, touched)
			if !nowOpaque {
				// The cell is air now; it can be re-lit by lit neighbours.
				relight = append(relight, litNeighbours(g, id)...)
			}
			propagateIncrease(g, relight, touched)
		}
	}

	switch {
	case newEmit > 0:
		// A new emitter seeds light.
		g.setLight(id, newEmit)
		propagateIncrease(g, []Vec3{id}, touched)
	case wasOpaque && !nowOpaque:
		// An opaque block was removed: block light flows into the opened cell.
		propagateIncrease(g, litNeighbours(g, id), touched)
	}
}

// litNeighbours returns the six neighbours of p that carry light in grid g.
func litNeighbours(g lightGrid, p Vec3) []Vec3 {
	var q []Vec3
	for _, d := range lightDirs {
		n := addVec(p, d)
		if g.loaded(n) && g.light(n) > 0 {
			q = append(q, n)
		}
	}
	return q
}
