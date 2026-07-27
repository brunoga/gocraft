//go:build aovisual

// Offscreen render verification for ambient occlusion. Gated behind the
// `aovisual` build tag so it never runs during normal `go test` (it needs a
// GL context). Run with:
//
//	PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig \
//	  go test -tags aovisual -run TestAORendersDarkening -v .
//
// It renders a single top face twice — once with corner occluders (AO on) and
// once with nothing occluding (AO off) — reads the framebuffer back, and checks
// that the AO render has a real brightness gradient with a dark corner while the
// flat render is uniform.

package main

import (
	"runtime"
	"testing"

	"github.com/faiface/glhf"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

const aoW, aoH = 128, 128

func TestAORendersDarkening(t *testing.T) {
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

	win, err := glfw.CreateWindow(aoW, aoH, "aotest", nil, nil)
	if err != nil {
		t.Skipf("cannot create GL window: %v", err)
	}
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatalf("gl.Init: %v", err)
	}

	shader, err := glhf.NewShader(glhf.AttrFormat{
		{Name: "pos", Type: glhf.Vec3},
		{Name: "tex", Type: glhf.Vec2},
		{Name: "normal", Type: glhf.Vec3},
		{Name: "ao", Type: glhf.Float},
		{Name: "light", Type: glhf.Float},
	}, glhf.AttrFormat{
		{Name: "matrix", Type: glhf.Mat4},
		{Name: "camera", Type: glhf.Vec3},
		{Name: "fogdis", Type: glhf.Float},
		{Name: "aoflag", Type: glhf.Float},
		{Name: "sundir", Type: glhf.Vec3},
		{Name: "daylight", Type: glhf.Float},
	}, blockVertexSource, withSkyCommon(blockFragmentSource))
	if err != nil {
		t.Fatalf("shader compile failed: %v", err)
	}

	// Solid mid-gray 2x2 texture, avoiding the (1,1,1) and (1,0,1) special cases
	// in the fragment shader.
	gray := make([]uint8, 2*2*4)
	for i := range gray {
		if i%4 == 3 {
			gray[i] = 255
		} else {
			gray[i] = 128
		}
	}
	texture := glhf.NewTexture(2, 2, false, gray)

	blockTex := &BlockTexture{}
	// Any texcoords work with a solid texture; use the atlas mapping for block 4.
	*blockTex = BlockTexture{
		Left: MakeFaceTexture(3), Right: MakeFaceTexture(3),
		Up: MakeFaceTexture(3), Down: MakeFaceTexture(3),
		Front: MakeFaceTexture(3), Back: MakeFaceTexture(3),
	}
	showTop := [6]bool{false, false, true, false, false, false}

	// Top-down orthographic view so the top face fills the viewport.
	proj := mgl32.Ortho(-0.55, 0.55, -0.55, 0.55, 0.1, 10)
	view := mgl32.LookAtV(mgl32.Vec3{0, 3, 0}, mgl32.Vec3{0, 0, 0}, mgl32.Vec3{0, 0, 1})
	matrix := proj.Mul4(view)

	render := func(occ func(dx, dy, dz int) bool, aoflag float32, lightAt func(dx, dy, dz int) float32, daylight float32) []uint8 {
		data := makeCubeData([]float32{}, showTop, Vec3{0, 0, 0}, blockTex, occ, lightAt)
		mesh := NewMesh(shader, data)
		defer mesh.Release()

		gl.Viewport(0, 0, aoW, aoH)
		gl.Enable(gl.DEPTH_TEST)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		shader.Begin()
		texture.Begin()
		shader.SetUniformAttr(0, matrix)
		shader.SetUniformAttr(1, mgl32.Vec3{0, 0, 0})
		shader.SetUniformAttr(2, float32(1000)) // huge fog distance -> no fog
		shader.SetUniformAttr(3, aoflag)
		shader.SetUniformAttr(4, mgl32.Vec3{0, 1, -0.3}.Normalize()) // sun overhead
		shader.SetUniformAttr(5, daylight)
		mesh.Draw()
		texture.End()
		shader.End()
		gl.Finish()

		buf := make([]uint8, aoW*aoH*4)
		gl.ReadPixels(0, 0, aoW, aoH, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&buf[0]))
		return buf
	}

	// Occluders on two edges of one corner -> that corner is fully dark (AO=0).
	occluded := occAt([3]int{1, 1, 0}, [3]int{0, 1, 1})
	aoPix := render(occluded, 1, lightFull, 1)     // AO on
	flatPix := render(aoOpen, 1, lightFull, 1)     // no occluders, AO on
	togglePix := render(occluded, 0, lightFull, 1) // same occluders, AO toggled off

	aoMin, aoMax, aoN := faceLumaStats(aoPix)
	flatMin, flatMax, flatN := faceLumaStats(flatPix)
	offMin, offMax, offN := faceLumaStats(togglePix)

	t.Logf("AO on   face pixels=%d luma min=%d max=%d", aoN, aoMin, aoMax)
	t.Logf("flat    face pixels=%d luma min=%d max=%d", flatN, flatMin, flatMax)
	t.Logf("AO off  face pixels=%d luma min=%d max=%d", offN, offMin, offMax)

	if aoN < 1000 || flatN < 1000 || offN < 1000 {
		t.Fatalf("face barely rendered (aoN=%d flatN=%d offN=%d); check projection", aoN, flatN, offN)
	}
	// Flat render is uniform.
	if flatMax-flatMin > 6 {
		t.Errorf("flat render should be uniform, got spread %d..%d", flatMin, flatMax)
	}
	// AO render has a real gradient...
	if aoMax-aoMin < 40 {
		t.Errorf("AO render should have a brightness gradient, got spread %d..%d", aoMin, aoMax)
	}
	// ...and its dark corner is clearly darker than the flat (fully lit) face.
	if int(aoMin) > int(flatMin)-30 {
		t.Errorf("AO dark corner (%d) not clearly darker than flat face (%d)", aoMin, flatMin)
	}
	// Toggling AO off makes the occluded geometry render uniformly again,
	// matching the flat face.
	if offMax-offMin > 6 {
		t.Errorf("AO-off render should be uniform, got spread %d..%d", offMin, offMax)
	}
	if offMin < flatMin-6 {
		t.Errorf("AO-off render (%d) should match the fully-lit flat face (%d)", offMin, flatMin)
	}

	// --- Skylight: the per-vertex light value scales surface brightness, and 0
	// (a fully enclosed cave) renders black. AO off to isolate the light term. ---
	lit := centerLuma(render(aoOpen, 0, lightConst(1.0), 1))
	dim := centerLuma(render(aoOpen, 0, lightConst(0.4), 1))
	dark := centerLuma(render(aoOpen, 0, lightConst(0.0), 1))
	t.Logf("skylight centre luma: lit=%d dim=%d dark=%d", lit, dim, dark)

	if lit < 60 {
		t.Errorf("fully-lit face too dark: luma=%d", lit)
	}
	if dark > 6 {
		t.Errorf("unlit (cave) face should be ~black, got luma=%d", dark)
	}
	if !(dim > dark+15 && dim < lit-15) {
		t.Errorf("dim light should sit between dark and lit: dark=%d dim=%d lit=%d", dark, dim, lit)
	}

	// --- Day-night: the daylight level scales sky-lit surfaces. A sky-exposed
	// face is much darker at night than at noon, but not black (moonlight);
	// a cave (light 0) is black at any daylight level. ---
	noon := centerLuma(render(aoOpen, 0, lightConst(1.0), 1.0))
	night := centerLuma(render(aoOpen, 0, lightConst(1.0), 0.1))
	caveNight := centerLuma(render(aoOpen, 0, lightConst(0.0), 0.1))
	t.Logf("day-night centre luma: noon=%d night=%d caveNight=%d", noon, night, caveNight)

	if night >= noon/2 {
		t.Errorf("night surface (%d) should be much darker than noon (%d)", night, noon)
	}
	if night < 2 {
		t.Errorf("night surface should keep faint moonlight, got %d", night)
	}
	if caveNight > 2 {
		t.Errorf("cave stays black regardless of daylight, got %d", caveNight)
	}
}

// centerLuma returns the luma of the centre pixel of the framebuffer (which the
// top face covers).
func centerLuma(buf []uint8) uint8 {
	i := ((aoH/2)*aoW + aoW/2) * 4
	r, g, b := int(buf[i]), int(buf[i+1]), int(buf[i+2])
	return uint8((r*30 + g*59 + b*11) / 100)
}

// lightConst returns a light sampler that always reports v.
func lightConst(v float32) func(dx, dy, dz int) float32 {
	return func(dx, dy, dz int) float32 { return v }
}

// faceLumaStats returns min/max luma and the count of non-background pixels
// (background was cleared to black).
func faceLumaStats(buf []uint8) (min, max uint8, n int) {
	min = 255
	for i := 0; i+3 < len(buf); i += 4 {
		r, g, b := int(buf[i]), int(buf[i+1]), int(buf[i+2])
		luma := uint8((r*30 + g*59 + b*11) / 100)
		if luma < 12 { // background
			continue
		}
		n++
		if luma < min {
			min = luma
		}
		if luma > max {
			max = luma
		}
	}
	if n == 0 {
		min = 0
	}
	return
}
