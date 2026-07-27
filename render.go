package main

import (
	"flag"
	"image"
	"image/draw"
	"log"
	"os"
	"sort"
	"sync"

	"github.com/faiface/glhf"
	"github.com/faiface/mainthread"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
)

var (
	texturePath  = flag.String("t", "texture.png", "texture file")
	renderRadius = flag.Int("r", 6, "render radius")
)

func loadImage(fname string) ([]uint8, image.Rectangle, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, image.Rectangle{}, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, image.Rectangle{}, err
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba.Pix, img.Bounds(), nil
}

type BlockRender struct {
	shader  *glhf.Shader
	texture *glhf.Texture

	facePool *sync.Pool

	sigch     chan struct{}
	meshcache sync.Map //map[Vec3]*Mesh

	stat Stat

	item *Mesh

	// aoEnabled toggles ambient occlusion at runtime via a shader uniform, so
	// no re-meshing is needed. AO data stays baked in the vertex buffers.
	aoEnabled bool
}

func NewBlockRender() (*BlockRender, error) {
	var (
		err error
	)
	img, rect, err := loadImage(*texturePath)
	if err != nil {
		return nil, err
	}

	r := &BlockRender{
		sigch:     make(chan struct{}, 4),
		aoEnabled: true,
	}

	mainthread.Call(func() {
		r.shader, err = glhf.NewShader(glhf.AttrFormat{
			glhf.Attr{Name: "pos", Type: glhf.Vec3},
			glhf.Attr{Name: "tex", Type: glhf.Vec2},
			glhf.Attr{Name: "normal", Type: glhf.Vec3},
			glhf.Attr{Name: "ao", Type: glhf.Float},
			glhf.Attr{Name: "light", Type: glhf.Float},
			glhf.Attr{Name: "blocklight", Type: glhf.Float},
		}, glhf.AttrFormat{
			glhf.Attr{Name: "matrix", Type: glhf.Mat4},
			glhf.Attr{Name: "camera", Type: glhf.Vec3},
			glhf.Attr{Name: "fogdis", Type: glhf.Float},
			glhf.Attr{Name: "aoflag", Type: glhf.Float},
			glhf.Attr{Name: "sundir", Type: glhf.Vec3},
			glhf.Attr{Name: "daylight", Type: glhf.Float},
		}, blockVertexSource, withSkyCommon(blockFragmentSource))

		if err != nil {
			return
		}
		r.texture = glhf.NewTexture(rect.Dx(), rect.Dy(), false, img)

	})
	if err != nil {
		return nil, err
	}
	r.facePool = &sync.Pool{
		New: func() interface{} {
			return make([]float32, 0, r.shader.VertexFormat().Size()/4*6*6)
		},
	}

	return r, nil
}

// buildChunkFaces builds a chunk's vertex data (the CPU-heavy part of meshing).
// It reads light and blocks under a read lock, using a per-call chunk cache so
// parallel workers don't contend on the LRU's lock. The returned buffer comes
// from facePool and must be handed to uploadChunkMesh, which returns it.
func (r *BlockRender) buildChunkFaces(c *Chunk) []float32 {
	facedata := r.facePool.Get().([]float32)

	w := game.world
	w.lightMu.RLock()
	defer w.lightMu.RUnlock()

	// A mesh only touches this chunk and its immediate neighbours, so a tiny
	// local cache turns almost every lookup into a map hit instead of a
	// lock-taking LRU peek. It is per-call, so nothing is shared between workers.
	cache := make(map[Vec3]*Chunk, 9)
	chunkAt := func(cid Vec3) *Chunk {
		if ch, ok := cache[cid]; ok {
			return ch
		}
		ch := w.peekChunk(cid)
		cache[cid] = ch
		return ch
	}
	blockAt := func(p Vec3) int {
		ch := chunkAt(p.Chunkid())
		if ch == nil {
			return -1
		}
		return ch.Block(p)
	}
	cellLight := func(p Vec3) uint8 {
		if p.Y >= WorldHeight {
			return MaxLight
		}
		if p.Y < 0 {
			return 0
		}
		ch := chunkAt(p.Chunkid())
		if ch == nil {
			return MaxLight
		}
		return ch.getLight(p)
	}
	cellBlockLight := func(p Vec3) uint8 {
		if p.Y < 0 || p.Y >= WorldHeight {
			return 0
		}
		ch := chunkAt(p.Chunkid())
		if ch == nil {
			return 0
		}
		return ch.getBlockLight(p)
	}

	c.RangeBlocks(func(id Vec3, tp int) {
		if tp == 0 {
			log.Panicf("unexpect 0 item type on %v", id)
		}
		// lightAt/blockAtLight return the sky/block light (0..1) of the
		// neighbouring air cell.
		lightAt := func(dx, dy, dz int) float32 {
			return float32(cellLight(Vec3{id.X + dx, id.Y + dy, id.Z + dz})) / float32(MaxLight)
		}
		blockAtLight := func(dx, dy, dz int) float32 {
			return float32(cellBlockLight(Vec3{id.X + dx, id.Y + dy, id.Z + dz})) / float32(MaxLight)
		}
		if isTorch(tp) {
			facedata = makeTorchData(facedata, id, tex.Texture(tp), lightAt, blockAtLight)
			return
		}
		show := [...]bool{
			IsTransparent(blockAt(id.Left())),
			IsTransparent(blockAt(id.Right())),
			IsTransparent(blockAt(id.Up())),
			IsTransparent(blockAt(id.Down())) && id.Y != 0,
			IsTransparent(blockAt(id.Front())),
			IsTransparent(blockAt(id.Back())),
		}
		if IsPlant(tp) || isFire(tp) {
			// Plants and fire render as cross-billboards.
			facedata = makePlantData(facedata, show, id, tex.Texture(tp), lightAt, blockAtLight)
		} else {
			// occ samples whether the neighbouring block at the given offset
			// occludes ambient light, used to compute per-vertex AO.
			occ := func(dx, dy, dz int) bool {
				return aoOccludes(blockAt(Vec3{id.X + dx, id.Y + dy, id.Z + dz}))
			}
			facedata = makeCubeData(facedata, show, id, tex.Texture(tp), occ, lightAt, blockAtLight)
		}
	})
	return facedata
}

// uploadChunkMesh turns face data into a GL mesh and caches it. It must run on
// the GL (main) thread and returns the face buffer to the pool.
func (r *BlockRender) uploadChunkMesh(c *Chunk, facedata []float32) {
	mesh := NewMesh(r.shader, facedata)
	mesh.Id = c.Id()
	r.meshcache.Store(c.Id(), mesh)
	r.facePool.Put(facedata[:0])
}

// buildChunksParallel builds vertex data for all chunks concurrently, returning
// one face buffer per chunk (aligned with chunks). The caller uploads them on
// the GL thread.
func (r *BlockRender) buildChunksParallel(chunks []*Chunk) [][]float32 {
	datas := make([][]float32, len(chunks))
	var wg sync.WaitGroup
	for i, c := range chunks {
		wg.Add(1)
		go func(i int, c *Chunk) {
			defer wg.Done()
			datas[i] = r.buildChunkFaces(c)
		}(i, c)
	}
	wg.Wait()
	return datas
}

// call on mainthread
func (r *BlockRender) UpdateItem(w int) {
	vertices := r.facePool.Get().([]float32)
	defer r.facePool.Put(vertices[:0])
	texture := tex.Texture(w)
	show := [...]bool{true, true, true, true, true, true}
	pos := Vec3{0, 0, 0}
	if isTorch(w) {
		vertices = makeTorchData(vertices, pos, texture, lightFull, blockNone)
	} else if IsPlant(w) || isFire(w) {
		vertices = makePlantData(vertices, show, pos, texture, lightFull, blockNone)
	} else {
		// The HUD preview has no world neighbours: nothing occludes it and it is
		// fully lit.
		vertices = makeCubeData(vertices, show, pos, texture, aoOpen, lightFull, blockNone)
	}
	item := NewMesh(r.shader, vertices)
	if r.item != nil {
		r.item.Release()
	}
	r.item = item
}

func frustumPlanes(mat *mgl32.Mat4) []mgl32.Vec4 {
	c1, c2, c3, c4 := mat.Rows()
	return []mgl32.Vec4{
		c4.Add(c1),          // left
		c4.Sub(c1),          // right
		c4.Sub(c2),          // top
		c4.Add(c2),          // bottom
		c4.Mul(0.1).Add(c3), // front
		c4.Mul(320).Sub(c3), // back
	}
}

func isChunkVisiable(planes []mgl32.Vec4, id Vec3) bool {
	p := mgl32.Vec3{float32(id.X * ChunkWidth), 0, float32(id.Z * ChunkWidth)}
	const m = ChunkWidth

	points := []mgl32.Vec3{
		mgl32.Vec3{p.X(), p.Y(), p.Z()},
		mgl32.Vec3{p.X() + m, p.Y(), p.Z()},
		mgl32.Vec3{p.X() + m, p.Y(), p.Z() + m},
		mgl32.Vec3{p.X(), p.Y(), p.Z() + m},

		mgl32.Vec3{p.X(), p.Y() + 256, p.Z()},
		mgl32.Vec3{p.X() + m, p.Y() + 256, p.Z()},
		mgl32.Vec3{p.X() + m, p.Y() + 256, p.Z() + m},
		mgl32.Vec3{p.X(), p.Y() + 256, p.Z() + m},
	}
	for _, plane := range planes {
		var in, out int
		for _, point := range points {
			if plane.Dot(point.Vec4(1)) < 0 {
				out++
			} else {
				in++
			}
			if in != 0 && out != 0 {
				break
			}
		}
		if in == 0 {
			return false
		}
	}
	return true
}

func (r *BlockRender) get3dmat() mgl32.Mat4 {
	n := float32(*renderRadius * ChunkWidth)
	width, height := game.win.GetSize()
	mat := mgl32.Perspective(radian(45), float32(width)/float32(height), 0.01, n)
	mat = mat.Mul4(game.camera.Matrix())
	return mat
}

func (r *BlockRender) get2dmat() mgl32.Mat4 {
	n := float32(*renderRadius * ChunkWidth)
	mat := mgl32.Ortho(-n, n, -n, n, -1, n)
	mat = mat.Mul4(game.camera.Matrix())
	return mat
}

func (r *BlockRender) sortChunks(chunks []Vec3) []Vec3 {
	cid := NearBlock(game.camera.Pos()).Chunkid()
	x, z := cid.X, cid.Z
	mat := r.get3dmat()
	planes := frustumPlanes(&mat)

	sort.Slice(chunks, func(i, j int) bool {
		v1 := isChunkVisiable(planes, chunks[i])
		v2 := isChunkVisiable(planes, chunks[j])
		if v1 && !v2 {
			return true
		}
		if v2 && !v1 {
			return false
		}
		d1 := (chunks[i].X-x)*(chunks[i].X-x) + (chunks[i].Z-z)*(chunks[i].Z-z)
		d2 := (chunks[j].X-x)*(chunks[j].X-x) + (chunks[j].Z-z)*(chunks[j].Z-z)
		return d1 < d2
	})
	return chunks
}

// neededChunks returns the set of chunk ids within the render radius of the
// camera (a disc in the X/Z plane).
func neededChunks() map[Vec3]bool {
	cid := NearBlock(game.camera.Pos()).Chunkid()
	n := *renderRadius
	needed := make(map[Vec3]bool)
	for dx := -n; dx < n; dx++ {
		for dz := -n; dz < n; dz++ {
			if dx*dx+dz*dz > n*n {
				continue
			}
			needed[Vec3{cid.X + dx, 0, cid.Z + dz}] = true
		}
	}
	return needed
}

func (r *BlockRender) updateMeshCache() {
	// Chunks whose light changed (from edits or neighbour loads) must be
	// re-meshed; mark their cached meshes dirty so the pass below rebuilds them.
	for _, id := range game.world.DrainLightDirty() {
		if mesh, ok := r.meshcache.Load(id); ok {
			mesh.(*Mesh).Dirty = true
		}
	}

	needed := neededChunks()
	var added, removed []Vec3
	r.meshcache.Range(func(k, v interface{}) bool {
		id := k.(Vec3)
		if !needed[id] {
			removed = append(removed, id)
			return true
		}
		return true
	})

	for id := range needed {
		mesh, ok := r.meshcache.Load(id)
		// 不在cache里面的需要重新构建
		if !ok {
			added = append(added, id)
		} else {
			if mesh.(*Mesh).Dirty {
				log.Printf("update cache %v", id)
				added = append(added, id)
				removed = append(removed, id)
			}
		}
	}
	// 单次并发构造的chunk个数
	const batchBuildChunk = 4
	r.sortChunks(added)
	if len(added) > 4 {
		added = added[:batchBuildChunk]
	}

	var removedMesh []*Mesh
	for _, id := range removed {
		log.Printf("remove cache %v", id)
		mesh, _ := r.meshcache.Load(id)
		r.meshcache.Delete(id)
		removedMesh = append(removedMesh, mesh.(*Mesh))
	}

	newChunks := game.world.Chunks(added)
	datas := r.buildChunksParallel(newChunks)
	mainthread.CallNonBlock(func() {
		for i, c := range newChunks {
			r.uploadChunkMesh(c, datas[i])
		}
	})

	mainthread.CallNonBlock(func() {
		for _, mesh := range removedMesh {
			mesh.Release()
		}
	})

}

// called on mainthread
// forceChunks is called on the main (GL) thread. It (re)builds the meshes for
// the given chunks that are missing or dirty, building face data in parallel
// and uploading inline (we are already on the GL thread, so we must not dispatch
// back to it).
func (r *BlockRender) forceChunks(ids []Vec3) {
	chunks := game.world.Chunks(ids)
	var build []*Chunk
	var removedMesh []*Mesh
	for _, chunk := range chunks {
		imesh, ok := r.meshcache.Load(chunk.Id())
		if ok && !imesh.(*Mesh).Dirty {
			continue
		}
		build = append(build, chunk)
		if ok {
			removedMesh = append(removedMesh, imesh.(*Mesh))
		}
	}
	datas := r.buildChunksParallel(build)
	for i, c := range build {
		r.uploadChunkMesh(c, datas[i])
	}
	for _, mesh := range removedMesh {
		mesh.Release()
	}
}

func (r *BlockRender) forcePlayerChunks() {
	bid := NearBlock(game.camera.Pos())
	cid := bid.Chunkid()
	var ids []Vec3
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			id := Vec3{cid.X + dx, 0, cid.Z + dz}
			ids = append(ids, id)
		}
	}
	r.forceChunks(ids)
}

func (r *BlockRender) checkChunks() {
	// nonblock signal
	select {
	case r.sigch <- struct{}{}:
	default:
	}
}

func (r *BlockRender) DirtyChunk(id Vec3) {
	mesh, ok := r.meshcache.Load(id)
	if !ok {
		return
	}
	mesh.(*Mesh).Dirty = true
}

func (r *BlockRender) UpdateLoop() {
	for {
		select {
		case <-r.sigch:
		}
		r.updateMeshCache()
	}
}

// ToggleAO enables or disables ambient occlusion at runtime and reports the new
// state. It only flips a flag read as a shader uniform each frame, so it takes
// effect immediately without rebuilding any chunk meshes.
func (r *BlockRender) ToggleAO() bool {
	r.aoEnabled = !r.aoEnabled
	return r.aoEnabled
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (r *BlockRender) drawChunks() {
	r.forcePlayerChunks()
	r.checkChunks()
	mat := r.get3dmat()

	r.shader.SetUniformAttr(0, mat)
	r.shader.SetUniformAttr(1, game.camera.Pos())
	r.shader.SetUniformAttr(2, float32(*renderRadius)*ChunkWidth)
	r.shader.SetUniformAttr(3, b2f(r.aoEnabled))
	// Day-night: sun direction, overall daylight level and sky colour for the
	// current time. game.prevtime is the frame's wall-clock time.
	now := game.prevtime
	r.shader.SetUniformAttr(4, gameClock.SunDir(now))
	r.shader.SetUniformAttr(5, gameClock.Daylight(now))

	planes := frustumPlanes(&mat)
	r.stat = Stat{}
	r.meshcache.Range(func(k, v interface{}) bool {
		id, mesh := k.(Vec3), v.(*Mesh)
		r.stat.CacheChunks++
		if isChunkVisiable(planes, id) {
			r.stat.RendingChunks++
			r.stat.Faces += mesh.Faces()
			mesh.Draw()
		}
		return true
	})
}

func (r *BlockRender) drawItem() {
	if r.item == nil {
		return
	}
	width, height := game.win.GetSize()
	ratio := float32(width) / float32(height)
	projection := mgl32.Ortho2D(0, 15, 0, 15/ratio)
	model := mgl32.Translate3D(1, 1, 0)
	model = model.Mul4(mgl32.HomogRotate3DX(radian(10)))
	model = model.Mul4(mgl32.HomogRotate3DY(radian(45)))
	mat := projection.Mul4(model)
	r.shader.SetUniformAttr(0, mat)
	r.shader.SetUniformAttr(1, mgl32.Vec3{0, 0, 0})
	r.shader.SetUniformAttr(2, float32(*renderRadius)*ChunkWidth)
	// Keep the HUD item preview stably lit regardless of the time of day: a
	// fixed sun angle and full daylight.
	r.shader.SetUniformAttr(4, mgl32.Vec3{-1, 1, -1}.Normalize())
	r.shader.SetUniformAttr(5, float32(1))
	r.item.Draw()
}

func (r *BlockRender) Draw() {
	r.shader.Begin()
	r.texture.Begin()

	r.drawChunks()
	r.drawItem()

	r.shader.End()
	r.texture.End()
}

type Stat struct {
	Faces         int
	CacheChunks   int
	RendingChunks int
}

func (r *BlockRender) Stat() Stat {
	return r.stat
}

type Mesh struct {
	vao, vbo uint32
	faces    int
	Id       Vec3
	Dirty    bool
}

func NewMesh(shader *glhf.Shader, data []float32) *Mesh {
	m := new(Mesh)
	m.faces = len(data) / (shader.VertexFormat().Size() / 4) / 6
	if m.faces == 0 {
		return m
	}
	gl.GenVertexArrays(1, &m.vao)
	gl.GenBuffers(1, &m.vbo)
	gl.BindVertexArray(m.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)

	offset := 0
	for _, attr := range shader.VertexFormat() {
		loc := gl.GetAttribLocation(shader.ID(), gl.Str(attr.Name+"\x00"))
		var size int32
		switch attr.Type {
		case glhf.Float:
			size = 1
		case glhf.Vec2:
			size = 2
		case glhf.Vec3:
			size = 3
		case glhf.Vec4:
			size = 4
		}
		gl.VertexAttribPointer(
			uint32(loc),
			size,
			gl.FLOAT,
			false,
			int32(shader.VertexFormat().Size()),
			gl.PtrOffset(offset),
		)
		gl.EnableVertexAttribArray(uint32(loc))
		offset += attr.Type.Size()
	}
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	return m
}

func (m *Mesh) Faces() int {
	return m.faces
}

func (m *Mesh) Draw() {
	if m.vao != 0 {
		gl.BindVertexArray(m.vao)
		gl.DrawArrays(gl.TRIANGLES, 0, int32(m.faces)*6)
		gl.BindVertexArray(0)
	}
}

func (m *Mesh) Release() {
	if m.vao != 0 {
		gl.DeleteVertexArrays(1, &m.vao)
		gl.DeleteBuffers(1, &m.vbo)
		m.vao = 0
		m.vbo = 0
	}
}

type Lines struct {
	vao, vbo uint32
	shader   *glhf.Shader
	nvertex  int
}

func NewLines(shader *glhf.Shader, data []float32) *Lines {
	l := new(Lines)
	l.shader = shader
	l.nvertex = len(data) / (shader.VertexFormat().Size() / 4)
	gl.GenVertexArrays(1, &l.vao)
	gl.GenBuffers(1, &l.vbo)
	gl.BindVertexArray(l.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, l.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)

	offset := 0
	for _, attr := range shader.VertexFormat() {
		loc := gl.GetAttribLocation(shader.ID(), gl.Str(attr.Name+"\x00"))
		var size int32
		switch attr.Type {
		case glhf.Float:
			size = 1
		case glhf.Vec2:
			size = 2
		case glhf.Vec3:
			size = 3
		case glhf.Vec4:
			size = 4
		}
		gl.VertexAttribPointer(
			uint32(loc),
			size,
			gl.FLOAT,
			false,
			int32(shader.VertexFormat().Size()),
			gl.PtrOffset(offset),
		)
		gl.EnableVertexAttribArray(uint32(loc))
		offset += attr.Type.Size()
	}
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	return l
}

func (l *Lines) Draw(mat mgl32.Mat4) {
	if l.vao != 0 {
		l.shader.SetUniformAttr(0, mat)
		gl.BindVertexArray(l.vao)
		gl.DrawArrays(gl.LINES, 0, int32(l.nvertex))
		gl.BindVertexArray(0)
	}
}

func (l *Lines) Release() {
	if l.vao != 0 {
		gl.DeleteVertexArrays(1, &l.vao)
		gl.DeleteBuffers(1, &l.vbo)
		l.vao = 0
		l.vbo = 0
	}
}

type LineRender struct {
	shader    *glhf.Shader
	cross     *Lines
	wireFrame *Lines
	lastBlock Vec3
}

func NewLineRender() (*LineRender, error) {
	r := &LineRender{}
	var err error
	mainthread.Call(func() {
		r.shader, err = glhf.NewShader(glhf.AttrFormat{
			glhf.Attr{Name: "pos", Type: glhf.Vec3},
		}, glhf.AttrFormat{
			glhf.Attr{Name: "matrix", Type: glhf.Mat4},
		}, lineVertexSource, lineFragmentSource)

		if err != nil {
			return
		}
		r.cross = makeCross(r.shader)
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *LineRender) drawCross() {
	width, height := game.win.GetFramebufferSize()
	project := mgl32.Ortho2D(0, float32(width), float32(height), 0)
	model := mgl32.Translate3D(float32(width/2), float32(height/2), 0)
	model = model.Mul4(mgl32.Scale3D(float32(height/30), float32(height/30), 0))
	r.cross.Draw(project.Mul4(model))
}

func (r *LineRender) drawWireFrame(mat mgl32.Mat4) {
	var vertices []float32
	block, _ := game.world.HitTest(game.camera.Pos(), game.camera.Front())
	if block == nil {
		return
	}

	mat = mat.Mul4(mgl32.Translate3D(float32(block.X), float32(block.Y), float32(block.Z)))
	mat = mat.Mul4(mgl32.Scale3D(1.06, 1.06, 1.06))
	if *block == r.lastBlock {
		r.wireFrame.Draw(mat)
		return
	}

	id := *block
	show := [...]bool{
		IsTransparent(game.world.Block(id.Left())),
		IsTransparent(game.world.Block(id.Right())),
		IsTransparent(game.world.Block(id.Up())),
		IsTransparent(game.world.Block(id.Down())),
		IsTransparent(game.world.Block(id.Front())),
		IsTransparent(game.world.Block(id.Back())),
	}
	vertices = makeWireFrameData(vertices, show)
	if len(vertices) == 0 {
		return
	}
	r.lastBlock = *block
	if r.wireFrame != nil {
		r.wireFrame.Release()
	}

	r.wireFrame = NewLines(r.shader, vertices)
	r.wireFrame.Draw(mat)
}

func (r *LineRender) Draw() {
	width, height := game.win.GetSize()
	projection := mgl32.Perspective(radian(45), float32(width)/float32(height), 0.01, ChunkWidth*float32(*renderRadius))
	camera := game.camera.Matrix()
	mat := projection.Mul4(camera)

	r.shader.Begin()
	r.drawCross()
	r.drawWireFrame(mat)
	r.shader.End()
}

func makeCross(shader *glhf.Shader) *Lines {
	return NewLines(shader, []float32{
		-0.5, 0, 0, 0.5, 0, 0,
		0, -0.5, 0, 0, 0.5, 0,
	})
}
