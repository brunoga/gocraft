package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestSmokeSpawnsFromSources(t *testing.T) {
	s := newSmokeSystem(1)
	src := []mgl32.Vec3{{0, 10, 0}}
	// One source at smokeSpawnPerSec should produce a handful of particles over
	// a second of simulation.
	for i := 0; i < 50; i++ {
		s.Step(src, 0.02)
	}
	if s.count() == 0 {
		t.Fatal("expected smoke particles to spawn from a source, got none")
	}
	if s.count() > smokeMaxParticles {
		t.Fatalf("particle count %d exceeds cap %d", s.count(), smokeMaxParticles)
	}
}

func TestSmokeNoSourcesNoSpawn(t *testing.T) {
	s := newSmokeSystem(1)
	for i := 0; i < 50; i++ {
		s.Step(nil, 0.02)
	}
	if s.count() != 0 {
		t.Fatalf("expected no particles without sources, got %d", s.count())
	}
}

func TestSmokeRises(t *testing.T) {
	s := newSmokeSystem(2)
	src := []mgl32.Vec3{{0, 10, 0}}
	// Seed some particles, then let them drift with no new spawns and confirm the
	// cloud's average height climbs.
	for i := 0; i < 10; i++ {
		s.Step(src, 0.02)
	}
	avg := func() float32 {
		var sum float32
		for i := range s.particles {
			sum += s.particles[i].pos[1]
		}
		return sum / float32(len(s.particles))
	}
	before := avg()
	for i := 0; i < 20; i++ {
		s.Step(nil, 0.05)
	}
	if s.count() == 0 {
		t.Fatal("particles expired too early to measure rise")
	}
	if after := avg(); after <= before {
		t.Fatalf("smoke should rise: avg height %.3f -> %.3f", before, after)
	}
}

func TestSmokeExpires(t *testing.T) {
	s := newSmokeSystem(3)
	src := []mgl32.Vec3{{0, 10, 0}}
	for i := 0; i < 30; i++ {
		s.Step(src, 0.02)
	}
	if s.count() == 0 {
		t.Fatal("expected particles before letting them expire")
	}
	// With no sources, every particle should age out within its maximum lifetime.
	maxLifeSeconds := float32(smokeLife) * 1.3
	steps := int(maxLifeSeconds/0.05) + 5
	for i := 0; i < steps; i++ {
		s.Step(nil, 0.05)
	}
	if s.count() != 0 {
		t.Fatalf("expected all smoke to dissipate, %d particles remain", s.count())
	}
}

func TestSmokeRespectsCap(t *testing.T) {
	s := newSmokeSystem(4)
	src := make([]mgl32.Vec3, 5000) // far more spawn demand than the cap allows
	for i := range src {
		src[i] = mgl32.Vec3{float32(i), 10, 0}
	}
	for i := 0; i < 100; i++ {
		s.Step(src, 0.02)
		if s.count() > smokeMaxParticles {
			t.Fatalf("particle count %d exceeded cap %d", s.count(), smokeMaxParticles)
		}
	}
	if s.count() != smokeMaxParticles {
		t.Fatalf("expected the cap %d to be saturated, got %d", smokeMaxParticles, s.count())
	}
}

func TestSmokeFadeShape(t *testing.T) {
	if f := smokeFade(0); f > 0.001 {
		t.Errorf("smoke should start transparent, got %.3f", f)
	}
	if f := smokeFade(1); f > 0.001 {
		t.Errorf("smoke should end transparent, got %.3f", f)
	}
	// Somewhere in the middle it should be substantially opaque.
	if f := smokeFade(0.2); f < 0.8 {
		t.Errorf("smoke should be near-opaque just after fade-in, got %.3f", f)
	}
	if f := smokeFade(0.15); f < 0.99 {
		t.Errorf("smoke should peak at the end of fade-in, got %.3f", f)
	}
}
