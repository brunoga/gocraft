package main

import (
	"github.com/faiface/glhf"
	"github.com/faiface/mainthread"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
)

// smokeFloatsPerParticle is the vertex stride: pos(3) + size(1) + alpha(1).
const smokeFloatsPerParticle = 5

// SmokeRender draws a SmokeSystem's particles as camera-facing point sprites.
// Each particle is one GL point whose pixel size is derived from its world size
// and depth in the vertex shader, and shaded as a soft round puff in the
// fragment shader (no texture needed). The vertex buffer is rebuilt from the
// live particles every frame.
type SmokeRender struct {
	shader   *glhf.Shader
	vao, vbo uint32
	buf      []float32 // scratch, reused each frame
}

func NewSmokeRender() (*SmokeRender, error) {
	var (
		r   *SmokeRender
		err error
	)
	mainthread.Call(func() {
		r, err = newSmokeGL()
	})
	return r, err
}

// newSmokeGL performs the GL setup on the current context/thread, separate from
// NewSmokeRender so tests can call it directly without the mainthread pump.
func newSmokeGL() (*SmokeRender, error) {
	shader, err := glhf.NewShader(glhf.AttrFormat{
		glhf.Attr{Name: "pos", Type: glhf.Vec3},
		glhf.Attr{Name: "size", Type: glhf.Float},
		glhf.Attr{Name: "alpha", Type: glhf.Float},
	}, glhf.AttrFormat{
		glhf.Attr{Name: "matrix", Type: glhf.Mat4},
		glhf.Attr{Name: "scale", Type: glhf.Float},
		glhf.Attr{Name: "daylight", Type: glhf.Float},
	}, smokeVertexSource, smokeFragmentSource)
	if err != nil {
		return nil, err
	}
	r := &SmokeRender{shader: shader}

	gl.GenVertexArrays(1, &r.vao)
	gl.GenBuffers(1, &r.vbo)
	gl.BindVertexArray(r.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	stride := int32(smokeFloatsPerParticle * 4)
	posLoc := uint32(gl.GetAttribLocation(shader.ID(), gl.Str("pos\x00")))
	gl.VertexAttribPointer(posLoc, 3, gl.FLOAT, false, stride, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(posLoc)
	sizeLoc := uint32(gl.GetAttribLocation(shader.ID(), gl.Str("size\x00")))
	gl.VertexAttribPointer(sizeLoc, 1, gl.FLOAT, false, stride, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(sizeLoc)
	alphaLoc := uint32(gl.GetAttribLocation(shader.ID(), gl.Str("alpha\x00")))
	gl.VertexAttribPointer(alphaLoc, 1, gl.FLOAT, false, stride, gl.PtrOffset(4*4))
	gl.EnableVertexAttribArray(alphaLoc)
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	return r, nil
}

// fill packs renderable puffs into r.buf as interleaved point-sprite vertices.
func (r *SmokeRender) fill(points []smokePoint) int {
	need := len(points) * smokeFloatsPerParticle
	if cap(r.buf) < need {
		r.buf = make([]float32, 0, need)
	}
	r.buf = r.buf[:0]
	for _, p := range points {
		r.buf = append(r.buf, p.pos[0], p.pos[1], p.pos[2], p.size, p.alpha)
	}
	return len(points)
}

// Draw renders a set of smoke puffs (from either smoke system). mat is the
// view-projection matrix, scale is viewport_height/(2*tan(fov/2)) for
// perspective sizing, and daylight (0..1) dims the smoke at night. It blends
// over the scene without writing depth, so puffs layer softly over terrain and
// each other.
func (r *SmokeRender) Draw(points []smokePoint, mat mgl32.Mat4, scale, daylight float32) {
	n := r.fill(points)
	if n == 0 {
		return
	}

	gl.Enable(gl.PROGRAM_POINT_SIZE)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DepthMask(false) // test against terrain, but don't occlude other puffs
	gl.Disable(gl.CULL_FACE)

	r.shader.Begin()
	r.shader.SetUniformAttr(0, mat)
	r.shader.SetUniformAttr(1, scale)
	r.shader.SetUniformAttr(2, daylight)
	gl.BindVertexArray(r.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(r.buf)*4, gl.Ptr(r.buf), gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.POINTS, 0, int32(n))
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	r.shader.End()

	gl.Enable(gl.CULL_FACE)
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.PROGRAM_POINT_SIZE)
}
