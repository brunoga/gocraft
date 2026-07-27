package main

import (
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	smokeMaxParticles = 3000 // hard cap on live particles
	smokeSpawnPerSec  = 10.0 // particles emitted per source per second
	smokeLife         = 3.2  // seconds a particle lives
	smokeRise         = 1.5  // upward speed (blocks/sec)
	smokeRiseAccel    = 0.4  // extra upward acceleration over the particle's life
	smokeDrift        = 0.5  // horizontal drift amplitude (blocks/sec)
	smokeStartSize    = 0.20 // billboard size (blocks) at birth
	smokeEndSize      = 1.7  // billboard size (blocks) at death
	smokeSpawnJitter  = 0.18 // random horizontal offset around a source at birth
)

// smokeParticle is a single rising smoke puff. seed decorrelates its drift and
// size wobble from its neighbours so a cluster does not move in lockstep.
type smokeParticle struct {
	pos  mgl32.Vec3
	vel  mgl32.Vec3
	age  float32
	life float32
	seed float32
}

// SmokeSystem is a CPU particle simulation for smoke rising from sources (fires
// and torches). It holds no GL state, so it is unit-testable on its own; the
// renderer reads the live particles each frame and uploads them as point
// sprites.
type SmokeSystem struct {
	particles []smokeParticle
	rng       *rand.Rand
	// pending accumulates fractional spawns across frames so a low per-source
	// rate still emits smoothly instead of rounding to zero every frame.
	pending float32
}

func newSmokeSystem(seed int64) *SmokeSystem {
	return &SmokeSystem{
		particles: make([]smokeParticle, 0, smokeMaxParticles),
		rng:       rand.New(rand.NewSource(seed)),
	}
}

// Step advances the simulation by dt seconds: it ages and moves existing
// particles (dropping the expired ones) and emits new particles from sources.
func (s *SmokeSystem) Step(sources []mgl32.Vec3, dt float32) {
	if dt <= 0 {
		return
	}
	s.advance(dt)
	s.spawn(sources, dt)
}

// advance moves and ages particles in place, compacting out the expired ones.
func (s *SmokeSystem) advance(dt float32) {
	live := s.particles[:0]
	for i := range s.particles {
		p := s.particles[i]
		p.age += dt
		if p.age >= p.life {
			continue
		}
		// Buoyancy: smoke accelerates upward slightly as it heats the air around
		// it, and drifts horizontally on a slow per-particle sine so a plume
		// curls instead of rising in a straight line.
		p.vel[1] += smokeRiseAccel * dt
		t := float64(p.seed*6.283 + p.age*1.7)
		p.vel[0] = smokeDrift * float32(math.Sin(t))
		p.vel[2] = smokeDrift * float32(math.Cos(t*0.9))
		p.pos = p.pos.Add(p.vel.Mul(dt))
		live = append(live, p)
	}
	s.particles = live
}

// spawn emits new particles from the given sources, honouring the global cap.
func (s *SmokeSystem) spawn(sources []mgl32.Vec3, dt float32) {
	if len(sources) == 0 {
		s.pending = 0
		return
	}
	s.pending += smokeSpawnPerSec * float32(len(sources)) * dt
	n := int(s.pending)
	if n <= 0 {
		return
	}
	s.pending -= float32(n)
	for i := 0; i < n; i++ {
		if len(s.particles) >= smokeMaxParticles {
			s.pending = 0
			return
		}
		src := sources[s.rng.Intn(len(sources))]
		s.particles = append(s.particles, smokeParticle{
			pos: mgl32.Vec3{
				src[0] + (s.rng.Float32()*2-1)*smokeSpawnJitter,
				src[1],
				src[2] + (s.rng.Float32()*2-1)*smokeSpawnJitter,
			},
			vel:  mgl32.Vec3{0, smokeRise, 0},
			life: smokeLife * (0.7 + 0.6*s.rng.Float32()),
			seed: s.rng.Float32(),
		})
	}
}

// count reports the number of live particles (for tests/diagnostics).
func (s *SmokeSystem) count() int { return len(s.particles) }

// smokeFade maps a particle's life fraction (0..1) to its opacity: smoke fades
// in quickly from nothing, holds, then fades out as it dissipates.
func smokeFade(frac float32) float32 {
	const in = 0.15
	switch {
	case frac < in:
		return frac / in
	default:
		return 1 - (frac-in)/(1-in)
	}
}
