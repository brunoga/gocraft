package main

import "testing"

func fakeWorld() (map[Vec3]int, func(Vec3) int, func(Vec3, int)) {
	m := map[Vec3]int{}
	get := func(p Vec3) int { return m[p] }
	set := func(p Vec3, tp int) {
		if tp == 0 {
			delete(m, p)
		} else {
			m[p] = tp
		}
	}
	return m, get, set
}

// Fire ignited on a line of wood should spread across all of it and eventually
// consume everything to air, leaving no fire burning.
func TestFireConsumesFuelAndBurnsOut(t *testing.T) {
	m, get, set := fakeWorld()
	for x := 0; x < 8; x++ {
		m[Vec3{x, 0, 0}] = 5 // wood
	}
	s := newFireSim(1)
	s.ignite(Vec3{0, 0, 0}, set)

	steps := 0
	for ; steps < 1000 && s.active() > 0; steps++ {
		s.tick(get, set)
	}
	if s.active() != 0 {
		t.Fatalf("fire never burned out after %d steps: %d cells still burning", steps, s.active())
	}
	for x := 0; x < 8; x++ {
		if got := m[Vec3{x, 0, 0}]; got != 0 {
			t.Errorf("wood at x=%d not consumed: block=%d", x, got)
		}
	}
}

// Fire with no flammable neighbour dies out within its lifetime.
func TestFireDiesWithoutFuel(t *testing.T) {
	m, get, set := fakeWorld()
	s := newFireSim(1)
	s.ignite(Vec3{5, 5, 5}, set)
	if m[Vec3{5, 5, 5}] != blockFire {
		t.Fatalf("ignite did not place fire")
	}
	for i := 0; i <= fireLife; i++ {
		s.tick(get, set)
	}
	if s.active() != 0 {
		t.Errorf("fire in open air should die out, %d still burning", s.active())
	}
	if m[Vec3{5, 5, 5}] != 0 {
		t.Errorf("burned-out fire should leave air, got block %d", m[Vec3{5, 5, 5}])
	}
}

// A fire removed by something else (e.g. the player) is dropped from the sim.
func TestFireRemovedExternally(t *testing.T) {
	m, get, set := fakeWorld()
	s := newFireSim(1)
	s.ignite(Vec3{1, 1, 1}, set)
	delete(m, Vec3{1, 1, 1}) // removed outside the sim
	s.tick(get, set)
	if s.active() != 0 {
		t.Errorf("externally-removed fire should be dropped, %d still tracked", s.active())
	}
}

// Fire emits block light and is transparent / non-solid.
func TestFireBlockProperties(t *testing.T) {
	if blockEmission(blockFire) != fireLight {
		t.Errorf("fire emission = %d, want %d", blockEmission(blockFire), fireLight)
	}
	if !IsTransparent(blockFire) {
		t.Error("fire should be transparent")
	}
	if IsObstacle(blockFire) {
		t.Error("fire should not be an obstacle")
	}
	if !isFlammable(5) || !isFlammable(15) || isFlammable(4) {
		t.Error("flammability: wood/leaves flammable, stone not")
	}
}
