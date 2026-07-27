//go:build aovisual

// Offscreen verification for the sky (gradient + sun + moon). Gated behind the
// `aovisual` build tag (needs a GL context). Run with:
//
//	PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig \
//	  go test -tags aovisual -run TestSky -v .
//
// It renders the fullscreen sky looking in chosen directions with chosen sun
// positions and reads back the centre pixel (which lies along the view ray).

package main

import (
	"runtime"
	"testing"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSkyRendersSunMoonGradient(t *testing.T) {
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
	win, err := glfw.CreateWindow(aoW, aoH, "skytest", nil, nil)
	if err != nil {
		t.Skipf("cannot create GL window: %v", err)
	}
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatalf("gl.Init: %v", err)
	}

	sky, err := newSkyGL()
	if err != nil {
		t.Fatalf("sky shader: %v", err)
	}

	up := mgl32.Vec3{0, 1, 0}
	// render points the camera along `front` with the given sun and daylight,
	// then returns the framebuffer. The centre pixel lies along `front`.
	render := func(front, sundir mgl32.Vec3, daylight float32) []uint8 {
		proj := mgl32.Perspective(mgl32.DegToRad(45), 1, 0.1, 100)
		view := mgl32.LookAtV(mgl32.Vec3{0, 0, 0}, front.Normalize(), up)
		invVP := proj.Mul4(view).Inv()

		gl.Viewport(0, 0, aoW, aoH)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		sky.Draw(invVP, mgl32.Vec3{0, 0, 0}, sundir.Normalize(), daylight)
		gl.Finish()

		buf := make([]uint8, aoW*aoH*4)
		gl.ReadPixels(0, 0, aoW, aoH, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&buf[0]))
		return buf
	}

	// 1) Looking straight at the daytime sun -> a near-white disc.
	sun := mgl32.Vec3{0.4, 0.5, 0.1}
	atSun := centerLuma(render(sun, sun, 1.0))
	if atSun < 220 {
		t.Errorf("looking at the sun should be near-white, got luma=%d", atSun)
	}

	// 2) East-west gradient at dusk: the sun's side of the horizon is warmer and
	// brighter than the opposite side.
	duskSun := mgl32.Vec3{1, 0.08, 0} // low in the +X sky
	sunSideR, _, _ := centerRGB(render(mgl32.Vec3{1, 0.05, 0}, duskSun, 0.5))
	antiR, _, _ := centerRGB(render(mgl32.Vec3{-1, 0.05, 0}, duskSun, 0.5))
	if sunSideR <= antiR {
		t.Errorf("dusk sun-side should be warmer (R=%d) than opposite (R=%d)", sunSideR, antiR)
	}
	sunSideLuma := centerLuma(render(mgl32.Vec3{1, 0.05, 0}, duskSun, 0.5))
	antiLuma := centerLuma(render(mgl32.Vec3{-1, 0.05, 0}, duskSun, 0.5))
	// The twilight gradient is strong: the anti-sun side is well under half the
	// sun side's brightness.
	if int(antiLuma)*2 >= int(sunSideLuma) {
		t.Errorf("dusk twilight gradient too weak: sun-side=%d anti-sun=%d", sunSideLuma, antiLuma)
	}
	// The anti-sun side is specifically darkened by twilight: the same view
	// direction is brighter under a high midday sun than at dusk.
	antiNoon := centerLuma(render(mgl32.Vec3{-1, 0.05, 0}, mgl32.Vec3{0, 1, -0.35}, 1.0))
	if antiLuma >= antiNoon {
		t.Errorf("anti-sun horizon should be darker at dusk (%d) than midday (%d)", antiLuma, antiNoon)
	}

	// 3) Night sky is much darker than day sky for the same upward view.
	upFront := mgl32.Vec3{0.2, 1, 0.2}
	daySky := centerLuma(render(upFront, mgl32.Vec3{0, 1, -0.35}, 1.0))
	nightSky := centerLuma(render(upFront, mgl32.Vec3{0, -1, 0.35}, 0.08))
	if nightSky >= daySky/2 {
		t.Errorf("night sky (%d) should be much darker than day (%d)", nightSky, daySky)
	}

	// 4) The moon (opposite the sun) is up at night: looking at it is brighter
	// than an empty patch of night sky.
	nightSun := mgl32.Vec3{0, -0.5, 0.2}
	moon := nightSun.Mul(-1)
	atMoon := centerLuma(render(moon, nightSun, 0.08))
	emptyNight := centerLuma(render(mgl32.Vec3{1, 0.3, 0}, nightSun, 0.08))
	if atMoon <= emptyNight {
		t.Errorf("moon (%d) should be brighter than empty night sky (%d)", atMoon, emptyNight)
	}

	t.Logf("sky luma: atSun=%d duskSun=%d duskAnti=%d day=%d night=%d atMoon=%d emptyNight=%d",
		atSun, sunSideLuma, antiLuma, daySky, nightSky, atMoon, emptyNight)
}

func centerRGB(buf []uint8) (r, g, b int) {
	i := ((aoH/2)*aoW + aoW/2) * 4
	return int(buf[i]), int(buf[i+1]), int(buf[i+2])
}
