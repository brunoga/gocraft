package main

import (
	"flag"
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

var dayLength = flag.Float64("daylen", 600, "length of a full day-night cycle in seconds")

// gameClock is the global day-night clock, initialised in run().
var gameClock *GameClock

// nightFloor is the minimum daylight level at night (moonlight). Fully enclosed
// caves are still black because their baked skylight is 0; this floor only keeps
// open, sky-exposed surfaces faintly visible after dark.
const nightFloor = 0.10

// GameClock maps wall-clock seconds to a time of day in [0,1): 0 = midnight,
// 0.25 = sunrise, 0.5 = noon, 0.75 = sunset. It carries no per-frame state; the
// sun direction, daylight level and sky colour are pure functions of the time.
type GameClock struct {
	length float64 // seconds per full cycle
	offset float64 // starting time-of-day
}

func NewGameClock(length float64) *GameClock {
	if length <= 0 {
		length = 600
	}
	return &GameClock{length: length, offset: 0.32} // start mid-morning
}

func (c *GameClock) TimeOfDay(now float64) float64 {
	t := now/c.length + c.offset
	return t - math.Floor(t)
}

func (c *GameClock) SunDir(now float64) mgl32.Vec3   { return sunDirAt(c.TimeOfDay(now)) }
func (c *GameClock) Daylight(now float64) float32    { return daylightAt(c.TimeOfDay(now)) }
func (c *GameClock) SkyColor(now float64) mgl32.Vec3 { return skyColorAt(c.TimeOfDay(now)) }

// sunDirAt returns the normalised direction from a surface toward the sun. The
// sun rises in -X, is overhead at noon, sets in +X, and is below the horizon at
// night (negative Y). The small constant Z tilts the arc so shading has depth.
func sunDirAt(t float64) mgl32.Vec3 {
	angle := (t - 0.25) * 2 * math.Pi
	return mgl32.Vec3{
		float32(-math.Cos(angle)),
		float32(math.Sin(angle)),
		-0.35,
	}.Normalize()
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// daylightAt returns the overall sky illumination for the time of day, ramping
// smoothly between the night floor and full daylight around dawn and dusk. It
// is driven by the sun's elevation (its Y component).
func daylightAt(t float64) float32 {
	elev := sunDirAt(t).Y()
	day := clamp01((elev + 0.15) / 0.35)
	return nightFloor + (1-nightFloor)*day
}

var (
	skyDay    = mgl32.Vec3{0.57, 0.71, 0.77}
	skyNight  = mgl32.Vec3{0.02, 0.03, 0.08}
	skySunset = mgl32.Vec3{0.85, 0.45, 0.25}
)

func mixVec3(a, b mgl32.Vec3, t float32) mgl32.Vec3 {
	return a.Add(b.Sub(a).Mul(t))
}

// skyColorAt returns the sky/fog colour for the time of day: dark at night,
// blue in the day, with an orange glow near the horizon at dawn and dusk.
func skyColorAt(t float64) mgl32.Vec3 {
	elev := sunDirAt(t).Y()
	day := clamp01((elev + 0.1) / 0.3)
	base := mixVec3(skyNight, skyDay, day)
	glow := clamp01(1 - float32(math.Abs(float64(elev)))/0.2)
	return mixVec3(base, skySunset, glow*0.7)
}
