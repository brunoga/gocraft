package main

import (
	"github.com/faiface/glhf"
	"github.com/faiface/mainthread"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
)

// SkyRender draws the sky as a fullscreen pass: a gradient (horizon to zenith,
// day to night, plus a warm glow on the sun's side of the horizon) with sun and
// moon discs. The fragment shader rebuilds the world-space view ray per pixel
// from the inverse view-projection matrix, so the sky and celestial bodies sit
// at infinity and track the sun direction as time advances.
type SkyRender struct {
	shader   *glhf.Shader
	vao, vbo uint32
}

func NewSkyRender() (*SkyRender, error) {
	var (
		r   *SkyRender
		err error
	)
	mainthread.Call(func() {
		r, err = newSkyGL()
	})
	return r, err
}

// newSkyGL performs the GL setup on the current context/thread. It is separate
// from NewSkyRender so tests can call it directly without the mainthread pump.
func newSkyGL() (*SkyRender, error) {
	r := &SkyRender{}
	shader, err := glhf.NewShader(glhf.AttrFormat{
		glhf.Attr{Name: "pos", Type: glhf.Vec2},
	}, glhf.AttrFormat{
		glhf.Attr{Name: "invvp", Type: glhf.Mat4},
		glhf.Attr{Name: "campos", Type: glhf.Vec3},
		glhf.Attr{Name: "sundir", Type: glhf.Vec3},
		glhf.Attr{Name: "daylight", Type: glhf.Float},
	}, skyVertexSource, withSkyCommon(skyFragmentSource))
	if err != nil {
		return nil, err
	}
	r.shader = shader

	// Two triangles covering clip space.
	quad := []float32{
		-1, -1, 1, -1, 1, 1,
		-1, -1, 1, 1, -1, 1,
	}
	gl.GenVertexArrays(1, &r.vao)
	gl.GenBuffers(1, &r.vbo)
	gl.BindVertexArray(r.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(quad)*4, gl.Ptr(quad), gl.STATIC_DRAW)
	loc := gl.GetAttribLocation(r.shader.ID(), gl.Str("pos\x00"))
	gl.VertexAttribPointer(uint32(loc), 2, gl.FLOAT, false, 2*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(uint32(loc))
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	return r, nil
}

// Draw renders the sky over the whole framebuffer. It must run before the
// terrain. invVP is the inverse of the camera's view-projection matrix.
func (r *SkyRender) Draw(invVP mgl32.Mat4, campos, sundir mgl32.Vec3, daylight float32) {
	// The sky fills the screen and must not occlude or be occluded via depth,
	// nor be back-face culled.
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)

	r.shader.Begin()
	r.shader.SetUniformAttr(0, invVP)
	r.shader.SetUniformAttr(1, campos)
	r.shader.SetUniformAttr(2, sundir)
	r.shader.SetUniformAttr(3, daylight)
	gl.BindVertexArray(r.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
	r.shader.End()

	gl.Enable(gl.CULL_FACE)
	gl.Enable(gl.DEPTH_TEST)
}
