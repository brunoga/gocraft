package main

// blockTorch is an upright, emissive, non-solid block that lights its
// surroundings. The blockTorch* variants are the same torch mounted on a wall,
// leaning away from it; the player only ever selects blockTorch, and placement
// swaps in the right variant from the clicked face (see orientTorch). Each
// variant is its own persisted block type so its orientation survives a reload.
const (
	blockTorch   = 65
	blockTorchXp = 67 // leans toward +X (wall on the -X side)
	blockTorchXn = 68 // leans toward -X
	blockTorchZp = 69 // leans toward +Z
	blockTorchZn = 70 // leans toward -Z
)

// torchLight is the block-light level a torch emits.
const torchLight = 14

// Wall-torch lean geometry: how far the tip shears sideways per unit of height,
// and how far the base is shifted toward the wall it hangs on.
const (
	torchTiltPerHeight = 0.9
	torchWallShift     = 0.42
)

// blockEmission returns the block-light level a block type emits (0 if none).
func blockEmission(tp int) uint8 {
	if isTorch(tp) {
		return torchLight
	}
	if tp == blockFire {
		return fireLight
	}
	return 0
}

func isTorch(tp int) bool {
	switch tp {
	case blockTorch, blockTorchXp, blockTorchXn, blockTorchZp, blockTorchZn:
		return true
	}
	return false
}

// torchLean returns the horizontal (dx,dz) unit direction a wall torch leans,
// or (0,0) for an upright (floor) torch.
func torchLean(tp int) (float32, float32) {
	switch tp {
	case blockTorchXp:
		return 1, 0
	case blockTorchXn:
		return -1, 0
	case blockTorchZp:
		return 0, 1
	case blockTorchZn:
		return 0, -1
	}
	return 0, 0
}

// torchOffsetAt returns the horizontal shear (dx,dz) applied to a torch vertex
// at height localY (relative to the block centre): the base sits against the
// wall and the tip leans out. For an upright torch it is always (0,0).
func torchOffsetAt(tp int, localY float32) (float32, float32) {
	lx, lz := torchLean(tp)
	s := (localY+0.5)*torchTiltPerHeight - torchWallShift
	return lx * s, lz * s
}

// orientTorch chooses the torch variant for a placement: the torch goes in cell
// torchCell against the adjacent block wall. A horizontal neighbour gives a wall
// torch leaning out from that wall; a floor or ceiling gives an upright torch.
func orientTorch(wall, torchCell Vec3) int {
	switch {
	case torchCell.X-wall.X == 1:
		return blockTorchXp
	case torchCell.X-wall.X == -1:
		return blockTorchXn
	case torchCell.Z-wall.Z == 1:
		return blockTorchZp
	case torchCell.Z-wall.Z == -1:
		return blockTorchZn
	}
	return blockTorch
}

// torchSupport returns the block a torch at pos is attached to: the wall behind
// a wall torch, or the floor below an upright one. Removing that block should
// remove the torch.
func torchSupport(pos Vec3, tp int) Vec3 {
	lx, lz := torchLean(tp)
	if lx == 0 && lz == 0 {
		return pos.Down()
	}
	return Vec3{pos.X - int(lx), pos.Y, pos.Z - int(lz)}
}

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
