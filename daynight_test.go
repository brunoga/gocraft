package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func brightness(c mgl32.Vec3) float32 { return c.X() + c.Y() + c.Z() }

func TestTimeOfDayWraps(t *testing.T) {
	c := &GameClock{length: 100, offset: 0}
	cases := map[float64]float64{
		0:   0.0,
		25:  0.25,
		50:  0.5,
		100: 0.0, // full cycle wraps
		150: 0.5,
	}
	for now, want := range cases {
		if got := c.TimeOfDay(now); math.Abs(got-want) > 1e-6 {
			t.Errorf("TimeOfDay(%v) = %v, want %v", now, got, want)
		}
	}
}

func TestSunElevation(t *testing.T) {
	// The sun is high at noon and below the horizon at midnight.
	if y := sunDirAt(0.5).Y(); y < 0.5 {
		t.Errorf("noon sun elevation = %v, want high (>0.5)", y)
	}
	if y := sunDirAt(0.0).Y(); y > -0.5 {
		t.Errorf("midnight sun elevation = %v, want low (<-0.5)", y)
	}
	// Near the horizon at sunrise and sunset.
	if y := sunDirAt(0.25).Y(); math.Abs(float64(y)) > 0.15 {
		t.Errorf("sunrise sun elevation = %v, want near 0", y)
	}
	if y := sunDirAt(0.75).Y(); math.Abs(float64(y)) > 0.15 {
		t.Errorf("sunset sun elevation = %v, want near 0", y)
	}
	// The sun rises in -X (dawn) and sets in +X (dusk).
	if sunDirAt(0.25).X() >= 0 {
		t.Errorf("sunrise should be toward -X, got X=%v", sunDirAt(0.25).X())
	}
	if sunDirAt(0.75).X() <= 0 {
		t.Errorf("sunset should be toward +X, got X=%v", sunDirAt(0.75).X())
	}
}

func TestDaylightLevel(t *testing.T) {
	if d := daylightAt(0.5); d < 0.95 {
		t.Errorf("noon daylight = %v, want ~1", d)
	}
	if d := daylightAt(0.0); math.Abs(float64(d-nightFloor)) > 1e-5 {
		t.Errorf("midnight daylight = %v, want nightFloor %v", d, nightFloor)
	}
	// Dawn sits between night and day.
	dawn := daylightAt(0.25)
	if !(dawn > nightFloor && dawn < 1) {
		t.Errorf("dawn daylight = %v, want between %v and 1", dawn, nightFloor)
	}
}

func TestSkyColor(t *testing.T) {
	night := skyColorAt(0.0)
	day := skyColorAt(0.5)
	// Day sky is much brighter than night sky.
	if brightness(day) <= brightness(night) {
		t.Errorf("day sky (%v) should be brighter than night (%v)", day, night)
	}
	if brightness(night) > 0.3 {
		t.Errorf("night sky too bright: %v", night)
	}
	// Dawn has a warm cast: red dominates green dominates blue.
	dawn := skyColorAt(0.25)
	if !(dawn.X() > dawn.Y() && dawn.Y() > dawn.Z()) {
		t.Errorf("dawn sky should be warm (R>G>B), got %v", dawn)
	}
}
