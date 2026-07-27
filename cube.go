package main

const (
	sleft = iota
	sright
	sup
	sdown
	sfront
	sback
)

// vertexAO returns the ambient-occlusion level (0 = fully occluded .. 3 = fully
// open) for a face corner given whether its two edge neighbours (side1, side2)
// and diagonal neighbour (corner) occlude light. When both edges are occluded
// the corner is fully dark regardless of the diagonal.
func vertexAO(side1, side2, corner bool) float32 {
	if side1 && side2 {
		return 0
	}
	return float32(3 - (b2i(side1) + b2i(side2) + b2i(corner)))
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// faceAO returns the AO level (0..3) for the six vertices of the given face, in
// the same order makeCubeData emits them. occ(dx, dy, dz) reports whether the
// block at that offset from the current block occludes light. For each corner
// the three sampled neighbours lie in the block layer directly in front of the
// face (one step along the face normal).
func faceAO(occ func(dx, dy, dz int) bool, face int) [6]float32 {
	switch face {
	case sleft: // -X, layer x-1, in-plane axes Y,Z
		ao := func(sy, sz int) float32 {
			return vertexAO(occ(-1, sy, 0), occ(-1, 0, sz), occ(-1, sy, sz))
		}
		return [6]float32{ao(-1, -1), ao(-1, 1), ao(1, 1), ao(1, 1), ao(1, -1), ao(-1, -1)}
	case sright: // +X, layer x+1, in-plane axes Y,Z
		ao := func(sy, sz int) float32 {
			return vertexAO(occ(1, sy, 0), occ(1, 0, sz), occ(1, sy, sz))
		}
		return [6]float32{ao(-1, 1), ao(-1, -1), ao(1, -1), ao(1, -1), ao(1, 1), ao(-1, 1)}
	case sup: // +Y, layer y+1, in-plane axes X,Z
		ao := func(sx, sz int) float32 {
			return vertexAO(occ(sx, 1, 0), occ(0, 1, sz), occ(sx, 1, sz))
		}
		return [6]float32{ao(-1, 1), ao(1, 1), ao(1, -1), ao(1, -1), ao(-1, -1), ao(-1, 1)}
	case sdown: // -Y, layer y-1, in-plane axes X,Z
		ao := func(sx, sz int) float32 {
			return vertexAO(occ(sx, -1, 0), occ(0, -1, sz), occ(sx, -1, sz))
		}
		return [6]float32{ao(-1, -1), ao(1, -1), ao(1, 1), ao(1, 1), ao(-1, 1), ao(-1, -1)}
	case sfront: // +Z, layer z+1, in-plane axes X,Y
		ao := func(sx, sy int) float32 {
			return vertexAO(occ(sx, 0, 1), occ(0, sy, 1), occ(sx, sy, 1))
		}
		return [6]float32{ao(-1, -1), ao(1, -1), ao(1, 1), ao(1, 1), ao(-1, 1), ao(-1, -1)}
	case sback: // -Z, layer z-1, in-plane axes X,Y
		ao := func(sx, sy int) float32 {
			return vertexAO(occ(sx, 0, -1), occ(0, sy, -1), occ(sx, sy, -1))
		}
		return [6]float32{ao(1, -1), ao(-1, -1), ao(-1, 1), ao(-1, 1), ao(1, 1), ao(1, -1)}
	}
	return [6]float32{3, 3, 3, 3, 3, 3}
}

// aoOpen is the occluder function to use when a block has no neighbourhood
// context (e.g. the HUD item preview): nothing occludes, so AO is fully open.
func aoOpen(dx, dy, dz int) bool { return false }

// show: left, right, up, down, front, back,
// occ(dx, dy, dz) reports whether the neighbouring block at that offset occludes
// ambient light; it is sampled per face corner to compute per-vertex AO.
func makeCubeData(vertices []float32, show [6]bool, block Vec3, tex *BlockTexture, occ func(dx, dy, dz int) bool, lightAt, blockAt func(dx, dy, dz int) float32) []float32 {
	l, r := tex.Left, tex.Right
	u, d := tex.Up, tex.Down
	f, b := tex.Front, tex.Back
	x, y, z := float32(block.X), float32(block.Y), float32(block.Z)
	// Each face samples the skylight of the air cell it faces (flat per face);
	// the value is the 10th float on every vertex of the face.
	if show[sleft] {
		a := faceAO(occ, sleft)
		lv := lightAt(-1, 0, 0)
		bv := blockAt(-1, 0, 0)
		vertices = append(vertices, []float32{
			// left
			x - 0.5, y - 0.5, z - 0.5, l[0][0], l[0][1], -1, 0, 0, a[0] / 3, lv, bv,
			x - 0.5, y - 0.5, z + 0.5, l[1][0], l[1][1], -1, 0, 0, a[1] / 3, lv, bv,
			x - 0.5, y + 0.5, z + 0.5, l[2][0], l[2][1], -1, 0, 0, a[2] / 3, lv, bv,
			x - 0.5, y + 0.5, z + 0.5, l[3][0], l[3][1], -1, 0, 0, a[3] / 3, lv, bv,
			x - 0.5, y + 0.5, z - 0.5, l[4][0], l[4][1], -1, 0, 0, a[4] / 3, lv, bv,
			x - 0.5, y - 0.5, z - 0.5, l[5][0], l[5][1], -1, 0, 0, a[5] / 3, lv, bv,
		}...)
	}
	if show[sright] {
		a := faceAO(occ, sright)
		lv := lightAt(1, 0, 0)
		bv := blockAt(1, 0, 0)
		vertices = append(vertices, []float32{
			// right
			x + 0.5, y - 0.5, z + 0.5, r[0][0], r[0][1], 1, 0, 0, a[0] / 3, lv, bv,
			x + 0.5, y - 0.5, z - 0.5, r[1][0], r[1][1], 1, 0, 0, a[1] / 3, lv, bv,
			x + 0.5, y + 0.5, z - 0.5, r[2][0], r[2][1], 1, 0, 0, a[2] / 3, lv, bv,
			x + 0.5, y + 0.5, z - 0.5, r[3][0], r[3][1], 1, 0, 0, a[3] / 3, lv, bv,
			x + 0.5, y + 0.5, z + 0.5, r[4][0], r[4][1], 1, 0, 0, a[4] / 3, lv, bv,
			x + 0.5, y - 0.5, z + 0.5, r[5][0], r[5][1], 1, 0, 0, a[5] / 3, lv, bv,
		}...)
	}
	if show[sup] {
		a := faceAO(occ, sup)
		lv := lightAt(0, 1, 0)
		bv := blockAt(0, 1, 0)
		vertices = append(vertices, []float32{
			// top
			x - 0.5, y + 0.5, z + 0.5, u[0][0], u[0][1], 0, 1, 0, a[0] / 3, lv, bv,
			x + 0.5, y + 0.5, z + 0.5, u[1][0], u[1][1], 0, 1, 0, a[1] / 3, lv, bv,
			x + 0.5, y + 0.5, z - 0.5, u[2][0], u[2][1], 0, 1, 0, a[2] / 3, lv, bv,
			x + 0.5, y + 0.5, z - 0.5, u[3][0], u[3][1], 0, 1, 0, a[3] / 3, lv, bv,
			x - 0.5, y + 0.5, z - 0.5, u[4][0], u[4][1], 0, 1, 0, a[4] / 3, lv, bv,
			x - 0.5, y + 0.5, z + 0.5, u[5][0], u[5][1], 0, 1, 0, a[5] / 3, lv, bv,
		}...)
	}

	if show[sdown] {
		a := faceAO(occ, sdown)
		lv := lightAt(0, -1, 0)
		bv := blockAt(0, -1, 0)
		vertices = append(vertices, []float32{
			// bottom
			x - 0.5, y - 0.5, z - 0.5, d[0][0], d[0][1], 0, -1, 0, a[0] / 3, lv, bv,
			x + 0.5, y - 0.5, z - 0.5, d[1][0], d[1][1], 0, -1, 0, a[1] / 3, lv, bv,
			x + 0.5, y - 0.5, z + 0.5, d[2][0], d[2][1], 0, -1, 0, a[2] / 3, lv, bv,
			x + 0.5, y - 0.5, z + 0.5, d[3][0], d[3][1], 0, -1, 0, a[3] / 3, lv, bv,
			x - 0.5, y - 0.5, z + 0.5, d[4][0], d[4][1], 0, -1, 0, a[4] / 3, lv, bv,
			x - 0.5, y - 0.5, z - 0.5, d[5][0], d[5][1], 0, -1, 0, a[5] / 3, lv, bv,
		}...)
	}

	if show[sfront] {
		a := faceAO(occ, sfront)
		lv := lightAt(0, 0, 1)
		bv := blockAt(0, 0, 1)
		vertices = append(vertices, []float32{
			// front
			x - 0.5, y - 0.5, z + 0.5, f[0][0], f[0][1], 0, 0, 1, a[0] / 3, lv, bv,
			x + 0.5, y - 0.5, z + 0.5, f[1][0], f[1][1], 0, 0, 1, a[1] / 3, lv, bv,
			x + 0.5, y + 0.5, z + 0.5, f[2][0], f[2][1], 0, 0, 1, a[2] / 3, lv, bv,
			x + 0.5, y + 0.5, z + 0.5, f[3][0], f[3][1], 0, 0, 1, a[3] / 3, lv, bv,
			x - 0.5, y + 0.5, z + 0.5, f[4][0], f[4][1], 0, 0, 1, a[4] / 3, lv, bv,
			x - 0.5, y - 0.5, z + 0.5, f[5][0], f[5][1], 0, 0, 1, a[5] / 3, lv, bv,
		}...)
	}

	if show[sback] {
		a := faceAO(occ, sback)
		lv := lightAt(0, 0, -1)
		bv := blockAt(0, 0, -1)
		vertices = append(vertices, []float32{
			// back
			x + 0.5, y - 0.5, z - 0.5, b[0][0], b[0][1], 0, 0, -1, a[0] / 3, lv, bv,
			x - 0.5, y - 0.5, z - 0.5, b[1][0], b[1][1], 0, 0, -1, a[1] / 3, lv, bv,
			x - 0.5, y + 0.5, z - 0.5, b[2][0], b[2][1], 0, 0, -1, a[2] / 3, lv, bv,
			x - 0.5, y + 0.5, z - 0.5, b[3][0], b[3][1], 0, 0, -1, a[3] / 3, lv, bv,
			x + 0.5, y + 0.5, z - 0.5, b[4][0], b[4][1], 0, 0, -1, a[4] / 3, lv, bv,
			x + 0.5, y - 0.5, z - 0.5, b[5][0], b[5][1], 0, 0, -1, a[5] / 3, lv, bv,
		}...)
	}

	return vertices
}

// lightFull is the light sampler for geometry with no world context (HUD item
// preview, player avatar): everything is fully lit.
func lightFull(dx, dy, dz int) float32 { return 1 }

func makeWireFrameData(vertices []float32, show [6]bool) []float32 {
	if show[sleft] {
		vertices = append(vertices, []float32{
			// left
			-0.5, -0.5, -0.5,
			-0.5, -0.5, +0.5,

			-0.5, -0.5, +0.5,
			-0.5, +0.5, +0.5,

			-0.5, +0.5, +0.5,
			-0.5, +0.5, -0.5,

			-0.5, +0.5, -0.5,
			-0.5, -0.5, -0.5,
		}...)
	}
	if show[sright] {
		vertices = append(vertices, []float32{
			// right
			+0.5, -0.5, +0.5,
			+0.5, -0.5, -0.5,

			+0.5, -0.5, -0.5,
			+0.5, +0.5, -0.5,

			+0.5, +0.5, -0.5,
			+0.5, +0.5, +0.5,

			+0.5, +0.5, +0.5,
			+0.5, -0.5, +0.5,
		}...)
	}

	if show[sup] {
		vertices = append(vertices, []float32{
			// top
			-0.5, +0.5, +0.5,
			+0.5, +0.5, +0.5,

			+0.5, +0.5, +0.5,
			+0.5, +0.5, -0.5,

			+0.5, +0.5, -0.5,
			-0.5, +0.5, -0.5,

			-0.5, +0.5, -0.5,
			-0.5, +0.5, +0.5,
		}...)
	}

	if show[sdown] {
		vertices = append(vertices, []float32{
			// bottom
			+0.5, -0.5, +0.5,
			-0.5, -0.5, +0.5,

			-0.5, -0.5, +0.5,
			-0.5, -0.5, -0.5,

			-0.5, -0.5, -0.5,
			+0.5, -0.5, -0.5,

			+0.5, -0.5, -0.5,
			+0.5, -0.5, +0.5,
		}...)
	}

	if show[sfront] {
		// z front
		vertices = append(vertices, []float32{
			-0.5, -0.5, +0.5,
			+0.5, -0.5, +0.5,

			+0.5, -0.5, +0.5,
			+0.5, +0.5, +0.5,

			+0.5, +0.5, +0.5,
			-0.5, +0.5, +0.5,

			-0.5, +0.5, +0.5,
			-0.5, -0.5, +0.5,
		}...)
	}

	if show[sback] {
		vertices = append(vertices, []float32{
			// back
			+0.5, -0.5, -0.5,
			-0.5, -0.5, -0.5,

			-0.5, -0.5, -0.5,
			-0.5, +0.5, -0.5,

			-0.5, +0.5, -0.5,
			+0.5, +0.5, -0.5,

			+0.5, +0.5, -0.5,
			+0.5, -0.5, -0.5,
		}...)
	}

	return vertices
}

func makePlantData(vertices []float32, show [6]bool, block Vec3, tex *BlockTexture, lightAt, blockAt func(dx, dy, dz int) float32) []float32 {
	l, r := tex.Left, tex.Right
	f, b := tex.Front, tex.Back
	x, y, z := float32(block.X), float32(block.Y), float32(block.Z)
	// Plants are cross-billboards with no axis-aligned faces adjacent to solid
	// neighbours, so ambient occlusion does not apply: emit full ao (1). They are
	// lit by the skylight of their own cell.
	lv := lightAt(0, 0, 0)
	bv := blockAt(0, 0, 0)
	vertices = append(vertices, []float32{
		// left
		x, y - 0.5, z - 0.5, l[0][0], l[0][1], -1, 0, 0, 1, lv, bv,
		x, y - 0.5, z + 0.5, l[1][0], l[1][1], -1, 0, 0, 1, lv, bv,
		x, y + 0.5, z + 0.5, l[2][0], l[2][1], -1, 0, 0, 1, lv, bv,
		x, y + 0.5, z + 0.5, l[3][0], l[3][1], -1, 0, 0, 1, lv, bv,
		x, y + 0.5, z - 0.5, l[4][0], l[4][1], -1, 0, 0, 1, lv, bv,
		x, y - 0.5, z - 0.5, l[5][0], l[5][1], -1, 0, 0, 1, lv, bv,
	}...)
	vertices = append(vertices, []float32{
		// right
		x, y - 0.5, z + 0.5, r[0][0], r[0][1], 1, 0, 0, 1, lv, bv,
		x, y - 0.5, z - 0.5, r[1][0], r[1][1], 1, 0, 0, 1, lv, bv,
		x, y + 0.5, z - 0.5, r[2][0], r[2][1], 1, 0, 0, 1, lv, bv,
		x, y + 0.5, z - 0.5, r[3][0], r[3][1], 1, 0, 0, 1, lv, bv,
		x, y + 0.5, z + 0.5, r[4][0], r[4][1], 1, 0, 0, 1, lv, bv,
		x, y - 0.5, z + 0.5, r[5][0], r[5][1], 1, 0, 0, 1, lv, bv,
	}...)

	vertices = append(vertices, []float32{
		// front
		x - 0.5, y - 0.5, z, f[0][0], f[0][1], 0, 0, 1, 1, lv, bv,
		x + 0.5, y - 0.5, z, f[1][0], f[1][1], 0, 0, 1, 1, lv, bv,
		x + 0.5, y + 0.5, z, f[2][0], f[2][1], 0, 0, 1, 1, lv, bv,
		x + 0.5, y + 0.5, z, f[3][0], f[3][1], 0, 0, 1, 1, lv, bv,
		x - 0.5, y + 0.5, z, f[4][0], f[4][1], 0, 0, 1, 1, lv, bv,
		x - 0.5, y - 0.5, z, f[5][0], f[5][1], 0, 0, 1, 1, lv, bv,
	}...)

	vertices = append(vertices, []float32{
		// back
		x + 0.5, y - 0.5, z, b[0][0], b[0][1], 0, 0, -1, 1, lv, bv,
		x - 0.5, y - 0.5, z, b[1][0], b[1][1], 0, 0, -1, 1, lv, bv,
		x - 0.5, y + 0.5, z, b[2][0], b[2][1], 0, 0, -1, 1, lv, bv,
		x - 0.5, y + 0.5, z, b[3][0], b[3][1], 0, 0, -1, 1, lv, bv,
		x + 0.5, y + 0.5, z, b[4][0], b[4][1], 0, 0, -1, 1, lv, bv,
		x + 0.5, y - 0.5, z, b[5][0], b[5][1], 0, 0, -1, 1, lv, bv,
	}...)
	return vertices
}

// makeTorchData builds a thin emissive post for a torch: always fully visible
// (no face culling or AO) and lit by its own cell (skylight + block light).
func makeTorchData(vertices []float32, block Vec3, tex *BlockTexture, lightAt, blockAt func(dx, dy, dz int) float32) []float32 {
	const w = 0.1
	t := tex.Up
	x, y, z := float32(block.X), float32(block.Y), float32(block.Z)
	x0, x1 := x-w, x+w
	y0, y1 := y-0.5, y+0.1
	z0, z1 := z-w, z+w
	lv := lightAt(0, 0, 0)
	bv := blockAt(0, 0, 0)
	vertices = append(vertices, []float32{
		// left (-x)
		x0, y0, z0, t[0][0], t[0][1], -1, 0, 0, 1, lv, bv,
		x0, y0, z1, t[1][0], t[1][1], -1, 0, 0, 1, lv, bv,
		x0, y1, z1, t[2][0], t[2][1], -1, 0, 0, 1, lv, bv,
		x0, y1, z1, t[3][0], t[3][1], -1, 0, 0, 1, lv, bv,
		x0, y1, z0, t[4][0], t[4][1], -1, 0, 0, 1, lv, bv,
		x0, y0, z0, t[5][0], t[5][1], -1, 0, 0, 1, lv, bv,
		// right (+x)
		x1, y0, z1, t[0][0], t[0][1], 1, 0, 0, 1, lv, bv,
		x1, y0, z0, t[1][0], t[1][1], 1, 0, 0, 1, lv, bv,
		x1, y1, z0, t[2][0], t[2][1], 1, 0, 0, 1, lv, bv,
		x1, y1, z0, t[3][0], t[3][1], 1, 0, 0, 1, lv, bv,
		x1, y1, z1, t[4][0], t[4][1], 1, 0, 0, 1, lv, bv,
		x1, y0, z1, t[5][0], t[5][1], 1, 0, 0, 1, lv, bv,
		// top (+y)
		x0, y1, z1, t[0][0], t[0][1], 0, 1, 0, 1, lv, bv,
		x1, y1, z1, t[1][0], t[1][1], 0, 1, 0, 1, lv, bv,
		x1, y1, z0, t[2][0], t[2][1], 0, 1, 0, 1, lv, bv,
		x1, y1, z0, t[3][0], t[3][1], 0, 1, 0, 1, lv, bv,
		x0, y1, z0, t[4][0], t[4][1], 0, 1, 0, 1, lv, bv,
		x0, y1, z1, t[5][0], t[5][1], 0, 1, 0, 1, lv, bv,
		// bottom (-y)
		x0, y0, z0, t[0][0], t[0][1], 0, -1, 0, 1, lv, bv,
		x1, y0, z0, t[1][0], t[1][1], 0, -1, 0, 1, lv, bv,
		x1, y0, z1, t[2][0], t[2][1], 0, -1, 0, 1, lv, bv,
		x1, y0, z1, t[3][0], t[3][1], 0, -1, 0, 1, lv, bv,
		x0, y0, z1, t[4][0], t[4][1], 0, -1, 0, 1, lv, bv,
		x0, y0, z0, t[5][0], t[5][1], 0, -1, 0, 1, lv, bv,
		// front (+z)
		x0, y0, z1, t[0][0], t[0][1], 0, 0, 1, 1, lv, bv,
		x1, y0, z1, t[1][0], t[1][1], 0, 0, 1, 1, lv, bv,
		x1, y1, z1, t[2][0], t[2][1], 0, 0, 1, 1, lv, bv,
		x1, y1, z1, t[3][0], t[3][1], 0, 0, 1, 1, lv, bv,
		x0, y1, z1, t[4][0], t[4][1], 0, 0, 1, 1, lv, bv,
		x0, y0, z1, t[5][0], t[5][1], 0, 0, 1, 1, lv, bv,
		// back (-z)
		x1, y0, z0, t[0][0], t[0][1], 0, 0, -1, 1, lv, bv,
		x0, y0, z0, t[1][0], t[1][1], 0, 0, -1, 1, lv, bv,
		x0, y1, z0, t[2][0], t[2][1], 0, 0, -1, 1, lv, bv,
		x0, y1, z0, t[3][0], t[3][1], 0, 0, -1, 1, lv, bv,
		x1, y1, z0, t[4][0], t[4][1], 0, 0, -1, 1, lv, bv,
		x1, y0, z0, t[5][0], t[5][1], 0, 0, -1, 1, lv, bv,
	}...)
	return vertices
}

// blockNone is a block-light sampler that reports no block light (for the HUD
// preview and player avatar).
func blockNone(dx, dy, dz int) float32 { return 0 }
