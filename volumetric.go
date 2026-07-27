package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	volMaxCells   = 6000 // hard cap on active smoke cells
	volInject     = 0.9  // density added to a source cell per tick
	volRise       = 0.5  // fraction of a cell's density that moves up per tick
	volDiffuse    = 0.2  // fraction that spreads to horizontal neighbours per tick
	volDecay      = 0.86 // density retained per tick (dissipation)
	volMinDensity = 0.03 // cells fainter than this are pruned / not drawn
	volCellSize   = 1.7  // billboard size (blocks) drawn per cell
	volAlpha      = 0.6  // opacity scale applied to a cell's density
	volTick       = 0.1  // seconds between simulation steps
)

// VolumetricSmoke is a cellular smoke simulation: smoke density lives on the
// integer block grid, and each tick every cell pushes some of its density up
// (buoyancy), spreads some to its horizontal neighbours (diffusion), and loses
// some to the air (dissipation). Unlike the particle system it fills a
// contiguous volume, so a plume reads as a soft cloud rather than discrete
// puffs. It holds no GL state and reaches the world only through source
// positions, so it is unit-testable on its own.
type VolumetricSmoke struct {
	density map[Vec3]float32
	next    map[Vec3]float32 // double buffer, reused each step
}

func newVolumetricSmoke() *VolumetricSmoke {
	return &VolumetricSmoke{
		density: make(map[Vec3]float32),
		next:    make(map[Vec3]float32),
	}
}

// cellOf returns the integer grid cell containing a world position.
func cellOf(p mgl32.Vec3) Vec3 {
	return Vec3{
		int(math.Floor(float64(p[0]))),
		int(math.Floor(float64(p[1]))),
		int(math.Floor(float64(p[2]))),
	}
}

// Step advances the simulation one tick: inject at the sources, then advect
// (rise), diffuse, and decay the whole field.
func (v *VolumetricSmoke) Step(sources []mgl32.Vec3) {
	for _, s := range sources {
		c := cellOf(s)
		if d := v.density[c] + volInject; d < 1 {
			v.density[c] = d
		} else {
			v.density[c] = 1
		}
	}

	next := v.next
	for k := range next {
		delete(next, k)
	}
	for c, d := range v.density {
		rise := d * volRise
		diff := d * volDiffuse
		next[c] += d - rise - diff
		next[Vec3{c.X, c.Y + 1, c.Z}] += rise
		next[Vec3{c.X - 1, c.Y, c.Z}] += diff * 0.25
		next[Vec3{c.X + 1, c.Y, c.Z}] += diff * 0.25
		next[Vec3{c.X, c.Y, c.Z - 1}] += diff * 0.25
		next[Vec3{c.X, c.Y, c.Z + 1}] += diff * 0.25
	}

	// Decay and prune faint cells in place.
	for c, d := range next {
		d *= volDecay
		if d < volMinDensity {
			delete(next, c)
		} else {
			next[c] = d
		}
	}
	// Backstop cap: if the field ever grows past the limit, raise the density
	// floor until it fits. Realistic emitter counts never reach this.
	if len(next) > volMaxCells {
		thresh := float32(volMinDensity)
		for len(next) > volMaxCells {
			thresh *= 1.5
			for c, d := range next {
				if d < thresh {
					delete(next, c)
				}
			}
		}
	}

	v.density, v.next = next, v.density
}

// count reports the number of active smoke cells (for tests/diagnostics).
func (v *VolumetricSmoke) count() int { return len(v.density) }

// total reports the summed density across all cells (for tests/diagnostics).
func (v *VolumetricSmoke) total() float32 {
	var sum float32
	for _, d := range v.density {
		sum += d
	}
	return sum
}

// appendPoints appends one renderable puff per active cell, centred in the cell
// with opacity scaled from its density.
func (v *VolumetricSmoke) appendPoints(dst []smokePoint) []smokePoint {
	for c, d := range v.density {
		a := d * volAlpha
		if a > 1 {
			a = 1
		}
		dst = append(dst, smokePoint{
			pos:   mgl32.Vec3{float32(c.X) + 0.5, float32(c.Y) + 0.5, float32(c.Z) + 0.5},
			size:  volCellSize,
			alpha: a,
		})
	}
	return dst
}
