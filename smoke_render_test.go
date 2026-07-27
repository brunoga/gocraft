//go:build aovisual

// Offscreen render verification for the particle smoke pass. Gated behind the
// `aovisual` build tag (it needs a GL context). Run with:
//
//	PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig \
//	  go test -tags aovisual -run TestSmokeRenders -v .
//
// It renders a cluster of smoke particles against a black background and checks
// that they produce visible grey coverage, that an empty system draws nothing,
// and that night dims the smoke relative to day.

package main

import (
	"runtime"
	"testing"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

const smokeW, smokeH = 128, 128

func TestSmokeRenders(t *testing.T) {
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		t.Skipf("no GL/display available: %v", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, gl.TRUE)
	glfw.WindowHint(glfw.Visible, glfw.False)

	win, err := glfw.CreateWindow(smokeW, smokeH, "smoketest", nil, nil)
	if err != nil {
		t.Skipf("cannot create GL window: %v", err)
	}
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatalf("gl.Init: %v", err)
	}

	r, err := newSmokeGL()
	if err != nil {
		t.Fatalf("smoke shader/GL setup failed: %v", err)
	}

	// Ortho view looking down -Z at a cluster of particles on the z=0 plane. With
	// ortho, clip-w is 1, so the point size is simply size*scale in pixels.
	proj := mgl32.Ortho(-2, 2, -2, 2, 0.1, 10)
	view := mgl32.LookAtV(mgl32.Vec3{0, 0, 3}, mgl32.Vec3{0, 0, 0}, mgl32.Vec3{0, 1, 0})
	matrix := proj.Mul4(view)
	const scale = 40

	// A grid of mid-life particles (frac ~0.2 -> nearly opaque, moderate size).
	seed := func() *SmokeSystem {
		s := newSmokeSystem(1)
		for x := -1.0; x <= 1.0; x += 0.5 {
			for y := -1.0; y <= 1.0; y += 0.5 {
				s.particles = append(s.particles, smokeParticle{
					pos:  mgl32.Vec3{float32(x), float32(y), 0},
					life: 1,
					age:  0.2,
				})
			}
		}
		return s
	}

	renderLuma := func(sys *SmokeSystem, daylight float32) (coverage int, avg float64) {
		gl.Viewport(0, 0, smokeW, smokeH)
		gl.Enable(gl.DEPTH_TEST)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		r.Draw(sys, matrix, scale, daylight)
		gl.Finish()

		buf := make([]uint8, smokeW*smokeH*4)
		gl.ReadPixels(0, 0, smokeW, smokeH, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&buf[0]))
		var sum float64
		for i := 0; i+3 < len(buf); i += 4 {
			rr, gg, bb := int(buf[i]), int(buf[i+1]), int(buf[i+2])
			luma := (rr*30 + gg*59 + bb*11) / 100
			if luma < 12 {
				continue // background
			}
			coverage++
			sum += float64(luma)
		}
		if coverage > 0 {
			avg = sum / float64(coverage)
		}
		return
	}

	// Daytime smoke: clearly visible grey coverage.
	dayCov, dayAvg := renderLuma(seed(), 1.0)
	t.Logf("day smoke: coverage=%d avg luma=%.1f", dayCov, dayAvg)
	if dayCov < 500 {
		t.Fatalf("smoke barely rendered: coverage=%d px (expected a visible cluster)", dayCov)
	}

	// Empty system: nothing drawn, frame stays black.
	emptyCov, _ := renderLuma(newSmokeSystem(1), 1.0)
	t.Logf("empty smoke: coverage=%d", emptyCov)
	if emptyCov != 0 {
		t.Errorf("empty smoke system should render nothing, got %d lit px", emptyCov)
	}

	// Night dims the smoke: same particles, lower average brightness.
	nightCov, nightAvg := renderLuma(seed(), 0.2)
	t.Logf("night smoke: coverage=%d avg luma=%.1f", nightCov, nightAvg)
	if nightCov < 500 {
		t.Fatalf("night smoke barely rendered: coverage=%d px", nightCov)
	}
	if nightAvg >= dayAvg {
		t.Errorf("night smoke (%.1f) should be dimmer than day smoke (%.1f)", nightAvg, dayAvg)
	}
}
