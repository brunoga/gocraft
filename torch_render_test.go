//go:build aovisual

// Offscreen render verification for the torch's three coloured segments and for
// the fire block. Gated behind `aovisual` (needs a GL context). Run with:
//
//	PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig \
//	  go test -tags aovisual -run TestTorchAndFireRender -v .

package main

import (
	"runtime"
	"testing"

	"github.com/faiface/glhf"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

const trW, trH = 128, 128

func TestTorchAndFireRender(t *testing.T) {
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
	win, err := glfw.CreateWindow(trW, trH, "torchtest", nil, nil)
	if err != nil {
		t.Skipf("cannot create GL window: %v", err)
	}
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatalf("gl.Init: %v", err)
	}
	if err := LoadTextureDesc(); err != nil {
		t.Fatalf("LoadTextureDesc: %v", err)
	}
	img, rect, err := loadImage("texture.png")
	if err != nil {
		t.Fatalf("loadImage: %v", err)
	}
	shader, err := glhf.NewShader(glhf.AttrFormat{
		{Name: "pos", Type: glhf.Vec3}, {Name: "tex", Type: glhf.Vec2},
		{Name: "normal", Type: glhf.Vec3}, {Name: "ao", Type: glhf.Float},
		{Name: "light", Type: glhf.Float}, {Name: "blocklight", Type: glhf.Float},
	}, glhf.AttrFormat{
		{Name: "matrix", Type: glhf.Mat4}, {Name: "camera", Type: glhf.Vec3},
		{Name: "fogdis", Type: glhf.Float}, {Name: "aoflag", Type: glhf.Float},
		{Name: "sundir", Type: glhf.Vec3}, {Name: "daylight", Type: glhf.Float},
	}, blockVertexSource, withSkyCommon(blockFragmentSource))
	if err != nil {
		t.Fatalf("shader: %v", err)
	}
	texture := glhf.NewTexture(rect.Dx(), rect.Dy(), false, img)

	full := func(dx, dy, dz int) float32 { return 1 }
	proj := mgl32.Perspective(radian(45), 1, 0.01, 100)
	view := mgl32.LookAtV(mgl32.Vec3{0, -0.15, 1.6}, mgl32.Vec3{0, -0.15, 0}, mgl32.Vec3{0, 1, 0})
	matrix := proj.Mul4(view)

	drawMesh := func(data []float32) []uint8 {
		mesh := NewMesh(shader, data)
		defer mesh.Release()
		gl.Viewport(0, 0, trW, trH)
		gl.Enable(gl.DEPTH_TEST)
		gl.Disable(gl.CULL_FACE)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		shader.Begin()
		texture.Begin()
		shader.SetUniformAttr(0, matrix)
		shader.SetUniformAttr(1, mgl32.Vec3{0, 0, 1.6})
		shader.SetUniformAttr(2, float32(1000))
		shader.SetUniformAttr(3, float32(0))
		shader.SetUniformAttr(4, mgl32.Vec3{0, 1, -0.3}.Normalize())
		shader.SetUniformAttr(5, float32(1))
		mesh.Draw()
		texture.End()
		shader.End()
		gl.Finish()
		buf := make([]uint8, trW*trH*4)
		gl.ReadPixels(0, 0, trW, trH, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&buf[0]))
		return buf
	}

	// Ratio-based colour classifiers (robust to the emissive brightness scaling).
	iabs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	isGrey := func(r, g, b int) bool { return iabs(r-g) < 22 && iabs(g-b) < 22 && r > 60 }
	// The ember/fire tile (burnt orange ~218,113,42) renders darker after shading;
	// classify by its strong red>green>blue spread rather than absolute brightness.
	isOrange := func(r, g, b int) bool { return r > 90 && r-g > 40 && g >= b }
	isBrown := func(r, g, b int) bool { return r > g && g > b && r-b > 15 && r < 210 }

	// --- Torch: expect brown (handle), grey (ash), and orange (ember) pixels. ---
	torch := drawMesh(makeTorchData(nil, Vec3{0, 0, 0}, blockTorch, tex.Texture(blockTorch), full, full))
	var brown, grey, orange, lit int
	for i := 0; i+3 < len(torch); i += 4 {
		r, g, b := int(torch[i]), int(torch[i+1]), int(torch[i+2])
		if r+g+b < 30 {
			continue
		}
		lit++
		switch {
		case isOrange(r, g, b):
			orange++
		case isGrey(r, g, b):
			grey++
		case isBrown(r, g, b):
			brown++
		}
	}
	t.Logf("torch: lit=%d brown=%d grey=%d orange=%d", lit, brown, grey, orange)
	if lit < 300 {
		t.Fatalf("torch barely rendered (lit=%d)", lit)
	}
	if brown < 30 {
		t.Errorf("torch handle should have brown pixels, got %d", brown)
	}
	if grey < 10 {
		t.Errorf("torch ash band should have grey pixels, got %d", grey)
	}
	if orange < 10 {
		t.Errorf("torch ember should have orange pixels, got %d", orange)
	}

	// --- Fire: should render as a visible orange billboard. ---
	show := [6]bool{true, true, true, true, true, true}
	fire := drawMesh(makePlantData(nil, show, Vec3{0, 0, 0}, tex.Texture(blockFire), full, full))
	var fireLit, fireOrange, fr, fg, fb int
	for i := 0; i+3 < len(fire); i += 4 {
		r, g, b := int(fire[i]), int(fire[i+1]), int(fire[i+2])
		if r+g+b < 30 {
			continue
		}
		fireLit++
		fr += r
		fg += g
		fb += b
		if isOrange(r, g, b) {
			fireOrange++
		}
	}
	if fireLit > 0 {
		t.Logf("fire: lit=%d orange=%d avg=(%d,%d,%d)", fireLit, fireOrange, fr/fireLit, fg/fireLit, fb/fireLit)
	}
	if fireLit < 300 {
		t.Fatalf("fire block did not render (lit=%d) -- it should show as an orange billboard", fireLit)
	}
	if fireOrange < 100 {
		t.Errorf("fire should be predominantly orange, got %d orange of %d lit", fireOrange, fireLit)
	}

	// --- Wall torch: a +X-leaning torch's ember (top) should sit to the +X side
	// (screen right, higher px) of its base (bottom). ReadPixels rows go
	// bottom-to-top, so the ember is in the upper rows. ---
	wall := drawMesh(makeTorchData(nil, Vec3{0, 0, 0}, blockTorchXp, tex.Texture(blockTorchXp), full, full))
	var topX, topN, botX, botN int
	for i := 0; i+3 < len(wall); i += 4 {
		r, g, b := int(wall[i]), int(wall[i+1]), int(wall[i+2])
		if r+g+b < 30 {
			continue
		}
		px := (i / 4) % trW
		py := (i / 4) / trW
		if py > trH/2 {
			topX += px
			topN++
		} else {
			botX += px
			botN++
		}
	}
	if topN == 0 || botN == 0 {
		t.Fatalf("wall torch barely rendered (top=%d bot=%d)", topN, botN)
	}
	topMean, botMean := topX/topN, botX/botN
	t.Logf("wall torch (+X lean): base meanX=%d ember meanX=%d", botMean, topMean)
	if topMean <= botMean+5 {
		t.Errorf("+X wall torch ember (meanX=%d) should lean right of its base (meanX=%d)", topMean, botMean)
	}
}
