package main

// Procedural world generator. All of the terrain shape lives here as pure
// functions of world coordinates so it can be unit-tested without any GL or
// chunk state: terrainHeight/surfaceBlocks decide the ground, caveCarved hollows
// it out, and treeInCell scatters forests. makeChunkMap (world.go) is the only
// caller; it just stamps the results into a chunk, clipping to the chunk bounds.
//
// The generator produces three headline landforms the old single-octave height
// field could not: rolling plains that thicken into forests, tall snow-capped
// mountain ridges, and 3-D cave systems winding through the rock below.

// Block types the generator places. These reference tiles in the texture atlas
// (see itemDesc): grass on soil, tan sand, grey stone, brown dirt, near-white
// snow, wood/leaves for trees and a white cloud block.
const (
	genGrass  = 1
	genSand   = 2
	genStone  = 3
	genWood   = 5
	genLeaves = 15
	genCloud  = 16
	genSnow   = 61
	genDirt   = 7
)

// Vertical layout of the world. WorldHeight (light.go) is 256, so mountains have
// room to tower and clouds float well above the highest peak.
const (
	genFloorTop     = 14  // lowest a surface column ever sits
	genSeaLevel     = 30  // baseline height of open lowland
	genBeachTop     = 33  // surfaces at or below this are sandy flats
	genRockLine     = 74  // mountain surfaces above this are bare stone
	genSnowLine     = 96  // ...and above this they are snow-capped
	genMountainPeak = 92  // how tall ridge lines rise above the base
	genSoilDepth    = 4   // dirt thickness beneath a grass surface
	genCaveFloor    = 3   // keep a solid floor below this (no bottomless caves)
	genCloudLow     = 176 // cloud layer, above the tallest peak
	genCloudHigh    = 186
)

// floorDiv is integer division rounding toward negative infinity (Go's / rounds
// toward zero), so cell indexing is continuous across x==0.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// hash2 is a cheap deterministic 2-D integer hash used to scatter trees without
// storing any state; equal inputs always give the same output.
func hash2(x, z int) uint32 {
	h := uint32(x)*0x1f1f1f1f ^ uint32(z)*0x85ebca6b
	h ^= h >> 15
	h *= 0x2c1b3c6d
	h ^= h >> 12
	h *= 0x297a2d39
	h ^= h >> 15
	return h
}

// terrainHeight returns the Y of the top solid block at column (x,z). It sums a
// broad "continental" swell, medium rolling hills, and sharp mountain ridges
// that only rise in the high-continent regions, so lowlands stay gentle while
// mountain country gets dramatic relief.
func terrainHeight(x, z int) int {
	fx, fz := float32(x), float32(z)

	// Continental elevation: very low frequency, decides land vs. highland.
	cont := noise2(fx*0.0016, fz*0.0016, 3, 0.5, 2) // 0..1
	// Rolling hills at medium scale.
	hills := (noise2(fx*0.02, fz*0.02, 4, 0.5, 2) - 0.5) * 12
	// Ridged mountain noise: 1-|2n-1| peaks along ridge lines; cubing sharpens
	// them into crests instead of round humps.
	mn := noise2(fx*0.0055, fz*0.0055, 5, 0.5, 2)
	ridge := 1 - abs(2*mn-1)
	ridge = ridge * ridge * ridge
	// Mountains only grow where the continent is already high, so ranges sit in
	// coherent regions rather than poking up everywhere.
	region := clamp01((cont - 0.5) / 0.28)

	h := float32(genSeaLevel) + cont*16 + hills + ridge*region*genMountainPeak
	hi := int(h)
	if hi < genFloorTop {
		hi = genFloorTop
	}
	return hi
}

// surfaceBlocks returns the top block and the block just beneath it for a column
// of height h: sand on low flats, grass over dirt on ordinary ground, bare stone
// on steep mountain rock, and snow over stone on the peaks. The snow and rock
// lines wobble with a little noise so they are not dead-flat contours.
func surfaceBlocks(x, z, h int) (surf, sub int) {
	if h <= genBeachTop {
		return genSand, genSand
	}
	snow := genSnowLine + int((noise2(float32(x)*0.01, float32(z)*0.01, 2, 0.5, 2)-0.5)*14)
	rock := genRockLine + int((noise2(float32(-x)*0.011, float32(-z)*0.011, 2, 0.5, 2)-0.5)*12)
	switch {
	case h >= snow:
		return genSnow, genStone
	case h >= rock:
		return genStone, genStone
	default:
		return genGrass, genDirt
	}
}

// columnBlock returns the block type at (x,y,z) for a solid column whose surface
// is at surfaceY: the surface material on top, a band of subsoil under it, then
// stone all the way down.
func columnBlock(x, z, y, surfaceY int) int {
	surf, sub := surfaceBlocks(x, z, surfaceY)
	switch {
	case y == surfaceY:
		return surf
	case y > surfaceY-genSoilDepth:
		return sub
	default:
		return genStone
	}
}

// caveCarved reports whether the rock at (x,y,z) should be hollowed into a cave.
// It intersects two independent 3-D noise fields near their mid-value: each
// field near 0.5 is a wavy sheet, and the intersection of two sheets is a set of
// winding tunnels, giving branching cave systems rather than round bubbles.
func caveCarved(x, y, z int) bool {
	if y < genCaveFloor {
		return false
	}
	fx, fy, fz := float32(x), float32(y), float32(z)
	a := noise3(fx*0.032, fy*0.05, fz*0.032, 2, 0.5, 2)
	b := noise3((fx+271)*0.032, (fy+131)*0.05, (fz+523)*0.032, 2, 0.5, 2)
	const t = 0.06
	return abs(a-0.5) < t && abs(b-0.5) < t
}

// Tree scattering. The world is tiled into treeCell-sized cells; each cell holds
// at most one tree at a hashed position, present with a probability that climbs
// steeply in forest regions. Because placement is a pure function of the cell
// index, neighbouring chunks agree on every tree that straddles their shared
// border, so canopies stitch seamlessly across chunk seams.
const treeCell = 5

// tree describes a single generated tree: its trunk base column, trunk height,
// and canopy radius.
type tree struct {
	x, z   int
	height int
	radius int
}

// treeInCell returns the tree for cell (cx,cz), if one grows there. A tree only
// takes root on a grass surface (not sand, rock or snow).
func treeInCell(cx, cz int) (tree, bool) {
	hsh := hash2(cx, cz)
	tx := cx*treeCell + int(hsh%treeCell)
	tz := cz*treeCell + int((hsh/treeCell)%treeCell)
	surfaceY := terrainHeight(tx, tz)
	if surf, _ := surfaceBlocks(tx, tz, surfaceY); surf != genGrass {
		return tree{}, false
	}
	// Forest density: dense forests raise the odds a cell is wooded; open plains
	// keep only the occasional lone tree.
	forest := noise2(float32(tx)*0.004+500, float32(tz)*0.004+500, 3, 0.5, 2)
	p := mix(0.05, 0.8, forest)
	if float32(hsh>>8)/float32(1<<24) >= p {
		return tree{}, false
	}
	height := 5 + int(hsh>>4&3) // 5..8
	return tree{x: tx, z: tz, height: height, radius: 2}, true
}

// blocks returns every block making up the tree, in world coordinates and
// independent of any chunk. Placement is a pure function of the tree, so two
// chunks sharing the tree derive exactly the same blocks and stitch its canopy
// seamlessly across their seam.
func (t tree) blocks() map[Vec3]int {
	out := make(map[Vec3]int)
	surfaceY := terrainHeight(t.x, t.z)
	top := surfaceY + t.height
	// Leafy canopy: a rounded blob around the trunk top.
	for dy := -2; dy <= 2; dy++ {
		r := t.radius
		if dy >= 1 {
			r-- // taper toward the crown
		}
		for ox := -r; ox <= r; ox++ {
			for oz := -r; oz <= r; oz++ {
				if ox*ox+oz*oz > r*r+1 {
					continue // round off the corners
				}
				out[Vec3{t.x + ox, top + dy, t.z + oz}] = genLeaves
			}
		}
	}
	// Trunk, written last so it is never hidden by its own leaves.
	for y := surfaceY + 1; y <= top; y++ {
		out[Vec3{t.x, y, t.z}] = genWood
	}
	return out
}

// addTree stamps a tree into m, keeping only blocks whose column falls inside
// the chunk [x0,x0+ChunkWidth) x [z0,z0+ChunkWidth). The caller runs this for
// every tree that could reach into the chunk, so a tree on a seam is completed
// by whichever chunk owns each of its blocks.
func addTree(m map[Vec3]int, t tree, x0, z0 int) {
	for p, tp := range t.blocks() {
		if p.X >= x0 && p.X < x0+ChunkWidth && p.Z >= z0 && p.Z < z0+ChunkWidth {
			m[p] = tp
		}
	}
}
