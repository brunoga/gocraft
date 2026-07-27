package main

import "math/rand"

const (
	blockFire = 66
	fireLight = 15  // block light a fire emits
	fireLife  = 6   // ticks a fire survives once it has no adjacent fuel
	fireTick  = 0.4 // seconds between simulation steps
)

// fireSpread is the chance per tick that a burning cell ignites one of its
// flammable neighbours.
const fireSpread = 0.45

func isFire(tp int) bool { return tp == blockFire }

// isFlammable reports whether fire can spread to / consume a block type.
func isFlammable(tp int) bool {
	return tp == 5 /* wood */ || tp == 15 /* leaves */ || IsPlant(tp)
}

func neighbors6(p Vec3) [6]Vec3 {
	return [6]Vec3{p.Left(), p.Right(), p.Up(), p.Down(), p.Front(), p.Back()}
}

// FireSim is a small cellular fire simulation: a burning cell clings to
// flammable blocks, spreads to adjacent flammable blocks (turning them to fire),
// and burns out once it has no fuel left. It reaches the world only through the
// get/set callbacks, so it is unit-testable against a plain map.
type FireSim struct {
	fires map[Vec3]int // active fire cells -> life remaining
	rng   *rand.Rand
}

func newFireSim(seed int64) *FireSim {
	return &FireSim{fires: map[Vec3]int{}, rng: rand.New(rand.NewSource(seed))}
}

// ignite starts a fire at pos. set changes the world block.
func (s *FireSim) ignite(pos Vec3, set func(Vec3, int)) {
	if _, ok := s.fires[pos]; ok {
		return
	}
	set(pos, blockFire)
	s.fires[pos] = fireLife
}

// tick advances the simulation one step. get reads a block type; set changes one.
func (s *FireSim) tick(get func(Vec3) int, set func(Vec3, int)) {
	// Snapshot the active cells so ignites during the tick take effect next tick.
	positions := make([]Vec3, 0, len(s.fires))
	for p := range s.fires {
		positions = append(positions, p)
	}
	for _, pos := range positions {
		if get(pos) != blockFire {
			delete(s.fires, pos) // the fire was removed elsewhere (e.g. the player)
			continue
		}
		var fuel []Vec3
		for _, n := range neighbors6(pos) {
			if isFlammable(get(n)) {
				fuel = append(fuel, n)
			}
		}
		if len(fuel) > 0 {
			s.fires[pos] = fireLife // refuelled: keep burning
			if s.rng.Float64() < fireSpread {
				s.ignite(fuel[s.rng.Intn(len(fuel))], set)
			}
		} else {
			s.fires[pos]--
			if s.fires[pos] <= 0 {
				delete(s.fires, pos)
				set(pos, 0) // burn out -> air
			}
		}
	}
}

// active reports how many fires are burning (for tests/diagnostics).
func (s *FireSim) active() int { return len(s.fires) }
