package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	_ "image/png"

	"net/http"
	_ "net/http/pprof"

	"github.com/faiface/mainthread"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

var (
	pprofPort = flag.String("pprof", "", "http pprof port")

	fullscreenSize = flag.String("fs", "", "fullscreen resolution as WxH (e.g. 1920x1080); empty uses the desktop resolution")

	game *Game
)

type Game struct {
	win *glfw.Window

	camera   *Camera
	lx, ly   float64
	vy       float32
	prevtime float64

	blockRender  *BlockRender
	lineRender   *LineRender
	playerRender *PlayerRender
	skyRender    *SkyRender
	smokeRender  *SmokeRender

	world   *World
	itemidx int
	item    int
	fps     FPS

	fireSim      *FireSim
	lastFireTick float64

	// Particle smoke rising from fires and torches. smokeSources is refreshed
	// on a throttled cadence (scanning nearby chunks is not free) and reused as
	// the emitter set every frame.
	smoke         *SmokeSystem
	smokeSources  []mgl32.Vec3
	lastSmokeScan float64

	exclusiveMouse bool
	closed         bool

	// Fullscreen state. When toggling back to windowed we restore the window
	// to the geometry saved in windowed*. The geometry is captured only once
	// (windowedSaved): re-reading it on every toggle would feed back the window
	// manager's decoration adjustments and shrink the window a little each time.
	fullscreen                                 bool
	windowedSaved                              bool
	windowedX, windowedY, windowedW, windowedH int
}

func initGL(w, h int) *glfw.Window {
	err := glfw.Init()
	if err != nil {
		log.Fatal(err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, gl.TRUE)

	win, err := glfw.CreateWindow(w, h, "gocraft", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	win.MakeContextCurrent()
	err = gl.Init()
	if err != nil {
		log.Fatal(err)
	}
	glfw.SwapInterval(1) // enable vsync
	gl.Enable(gl.DEPTH_TEST)
	gl.Enable(gl.CULL_FACE)
	return win
}

func NewGame(w, h int) (*Game, error) {
	var (
		err  error
		game *Game
	)
	game = new(Game)
	game.item = availableItems[0]
	game.fireSim = newFireSim(time.Now().UnixNano())
	game.smoke = newSmokeSystem(time.Now().UnixNano())

	mainthread.Call(func() {
		win := initGL(w, h)
		win.SetMouseButtonCallback(game.onMouseButtonCallback)
		win.SetCursorPosCallback(game.onCursorPosCallback)
		win.SetFramebufferSizeCallback(game.onFrameBufferSizeCallback)
		win.SetKeyCallback(game.onKeyCallback)
		game.win = win
	})
	game.world = NewWorld()
	game.camera = NewCamera(mgl32.Vec3{0, 16, 0})
	game.blockRender, err = NewBlockRender()
	if err != nil {
		return nil, err
	}
	mainthread.Call(func() {
		game.blockRender.UpdateItem(game.item)
	})
	game.lineRender, err = NewLineRender()
	if err != nil {
		return nil, err
	}
	game.playerRender, err = NewPlayerRender()
	if err != nil {
		return nil, err
	}
	game.skyRender, err = NewSkyRender()
	if err != nil {
		return nil, err
	}
	game.smokeRender, err = NewSmokeRender()
	if err != nil {
		return nil, err
	}
	go game.blockRender.UpdateLoop()
	go game.syncPlayerLoop()
	return game, nil
}

func (g *Game) setExclusiveMouse(exclusive bool) {
	if exclusive {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	} else {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
	g.exclusiveMouse = exclusive
}

// setSimBlock changes a block from the fire simulation: it updates lighting and
// marks the chunk for re-meshing, but does not persist (fire is transient).
func (g *Game) setSimBlock(pos Vec3, tp int) {
	old := g.world.Block(pos)
	if old == tp {
		return
	}
	g.world.setBlockTransient(pos, old, tp)
	g.dirtyBlock(pos)
}

// smokeScanTick is how often the smoke emitter set is rebuilt. Scanning nearby
// chunks for torches is not free, so it runs on this slower cadence, not every
// frame; the cached sources drive the per-frame particle sim in between.
const smokeScanTick = 0.4

// refreshSmokeSources rebuilds the list of smoke emitters: every active fire
// cell (positions are exact and free from the fire sim) plus every torch in the
// chunks around the player. It reuses the backing array to avoid allocating on
// each refresh.
func (g *Game) refreshSmokeSources() {
	// A block at (X,Y,Z) is centred on (X,Y,Z) in world space (it spans
	// X-0.5..X+0.5), so smoke rises from the block centre, not a corner.
	src := g.smokeSources[:0]
	for p := range g.fireSim.fires {
		src = append(src, mgl32.Vec3{float32(p.X), float32(p.Y) + 0.3, float32(p.Z)})
	}
	cid := NearBlock(g.camera.Pos()).Chunkid()
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			ch := g.world.peekChunk(Vec3{cid.X + dx, 0, cid.Z + dz})
			if ch == nil {
				continue
			}
			ch.RangeBlocks(func(id Vec3, w int) {
				if isTorch(w) {
					// Smoke leaves the ember, which leans out on a wall torch.
					ox, oz := torchOffsetAt(w, 0.2)
					src = append(src, mgl32.Vec3{float32(id.X) + ox, float32(id.Y) + 0.35, float32(id.Z) + oz})
				}
			})
		}
	}
	g.smokeSources = src
}

// smokeScale returns the perspective point-size factor for the current viewport:
// a world size multiplied by this and divided by clip-w gives a pixel size.
func (g *Game) smokeScale() float32 {
	_, h := g.win.GetSize()
	return float32(h) / (2 * float32(math.Tan(float64(radian(45))/2)))
}

// removeAttachedTorches removes any torch mounted on the block at p (an upright
// torch sitting on top of it, or a wall torch hanging on one of its sides), now
// that p is gone. It checks only the cells that could hold such a torch.
func (g *Game) removeAttachedTorches(p Vec3) {
	for _, n := range []Vec3{p.Up(), p.Left(), p.Right(), p.Front(), p.Back()} {
		tp := g.world.Block(n)
		if isTorch(tp) && torchSupport(n, tp) == p {
			g.world.UpdateBlock(n, 0)
			g.dirtyBlock(n)
			go ClientUpdateBlock(n, 0)
		}
	}
}

func (g *Game) dirtyBlock(id Vec3) {
	cid := id.Chunkid()
	g.blockRender.DirtyChunk(cid)
	neighbors := []Vec3{id.Left(), id.Right(), id.Front(), id.Back()}
	for _, neighbor := range neighbors {
		chunkid := neighbor.Chunkid()
		if chunkid != cid {
			g.blockRender.DirtyChunk(chunkid)
		}
	}
}

func (g *Game) onMouseButtonCallback(win *glfw.Window, button glfw.MouseButton, action glfw.Action, mod glfw.ModifierKey) {
	if !g.exclusiveMouse {
		g.setExclusiveMouse(true)
		return
	}
	head := NearBlock(g.camera.Pos())
	foot := head.Down()
	block, prev := g.world.HitTest(g.camera.Pos(), g.camera.Front())
	if button == glfw.MouseButton2 && action == glfw.Press {
		if prev != nil && *prev != head && *prev != foot {
			if isFire(g.item) {
				// Fire is a transient simulated block, not a persisted one.
				g.fireSim.ignite(*prev, g.setSimBlock)
			} else {
				tp := g.item
				if isTorch(tp) && block != nil {
					// Torches mount to the clicked surface: a wall gives a leaning
					// wall torch, a floor an upright one.
					tp = orientTorch(*block, *prev)
				}
				g.world.UpdateBlock(*prev, tp)
				g.dirtyBlock(*prev)
				go ClientUpdateBlock(*prev, tp)
			}
		}
	}
	if button == glfw.MouseButton1 && action == glfw.Press {
		if block != nil {
			g.world.UpdateBlock(*block, 0)
			g.dirtyBlock(*block)
			go ClientUpdateBlock(*block, 0)
			// A removed block no longer supports any torch attached to it.
			g.removeAttachedTorches(*block)
		}
	}
}

func (g *Game) onFrameBufferSizeCallback(window *glfw.Window, width, height int) {
	gl.Viewport(0, 0, int32(width), int32(height))
}

func (g *Game) onCursorPosCallback(win *glfw.Window, xpos float64, ypos float64) {
	if !g.exclusiveMouse {
		return
	}
	if g.lx == 0 && g.ly == 0 {
		g.lx, g.ly = xpos, ypos
		return
	}
	dx, dy := xpos-g.lx, g.ly-ypos
	g.lx, g.ly = xpos, ypos
	g.camera.OnAngleChange(float32(dx), float32(dy))
}

func (g *Game) onKeyCallback(win *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	if action != glfw.Press {
		return
	}
	switch key {
	case glfw.KeyF:
		// Alt+F toggles fullscreen.
		if mods&glfw.ModAlt != 0 {
			g.toggleFullscreen()
		}
	case glfw.KeyO:
		// O toggles ambient occlusion.
		on := g.blockRender.ToggleAO()
		log.Printf("ambient occlusion: %v", on)
	case glfw.KeyTab:
		g.camera.FlipFlying()
	case glfw.KeySpace:
		block := g.CurrentBlockid()
		if g.world.HasBlock(Vec3{block.X, block.Y - 2, block.Z}) {
			g.vy = 8
		}
	case glfw.KeyE:
		g.itemidx = (1 + g.itemidx) % len(availableItems)
		g.item = availableItems[g.itemidx]
		g.blockRender.UpdateItem(g.item)
	case glfw.KeyR:
		g.itemidx--
		if g.itemidx < 0 {
			g.itemidx = len(availableItems) - 1
		}
		g.item = availableItems[g.itemidx]
		g.blockRender.UpdateItem(g.item)
	}
}

// toggleFullscreen switches the window between windowed and fullscreen mode.
// It runs on the main thread (invoked from the GLFW key callback during
// PollEvents), so it can call GLFW window functions directly.
func (g *Game) toggleFullscreen() {
	if g.fullscreen {
		// Restore the previously saved windowed geometry.
		g.win.SetMonitor(nil, g.windowedX, g.windowedY, g.windowedW, g.windowedH, 0)
		g.fullscreen = false
		return
	}

	monitor := glfw.GetPrimaryMonitor()
	if monitor == nil {
		log.Print("fullscreen: no monitor available")
		return
	}
	mode := monitor.GetVideoMode()
	if mode == nil {
		log.Print("fullscreen: could not query video mode")
		return
	}

	// Default to the desktop resolution and refresh rate; override the size
	// if a valid -fs resolution was provided.
	width, height, refresh := mode.Width, mode.Height, mode.RefreshRate
	if *fullscreenSize != "" {
		if w, h, ok := parseResolution(*fullscreenSize); ok {
			width, height = w, h
		} else {
			log.Printf("fullscreen: invalid -fs value %q, using desktop resolution", *fullscreenSize)
		}
	}

	// Remember the windowed geometry the first time so we can return to it
	// later. Capturing it only once avoids accumulating window-manager
	// decoration drift across repeated toggles.
	if !g.windowedSaved {
		g.windowedX, g.windowedY = g.win.GetPos()
		g.windowedW, g.windowedH = g.win.GetSize()
		g.windowedSaved = true
	}

	g.win.SetMonitor(monitor, 0, 0, width, height, refresh)
	g.fullscreen = true
}

// parseResolution parses a "WxH" string (e.g. "1920x1080") into positive
// dimensions. ok is false if the string is malformed or non-positive.
func parseResolution(s string) (w, h int, ok bool) {
	n, err := fmt.Sscanf(s, "%dx%d", &w, &h)
	if err != nil || n != 2 || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func (g *Game) handleKeyInput(dt float64) {
	// Capture the position before movement so collision can be resolved along
	// the whole path this frame, not just at the destination.
	from := g.camera.Pos()
	speed := float32(0.1)
	if g.camera.flying {
		speed = 0.2
	}
	if g.win.GetKey(glfw.KeyEscape) == glfw.Press {
		g.setExclusiveMouse(false)
	}
	if g.win.GetKey(glfw.KeyW) == glfw.Press {
		g.camera.OnMoveChange(MoveForward, speed)
	}
	if g.win.GetKey(glfw.KeyS) == glfw.Press {
		g.camera.OnMoveChange(MoveBackward, speed)
	}
	if g.win.GetKey(glfw.KeyA) == glfw.Press {
		g.camera.OnMoveChange(MoveLeft, speed)
	}
	if g.win.GetKey(glfw.KeyD) == glfw.Press {
		g.camera.OnMoveChange(MoveRight, speed)
	}
	if g.win.GetKey(glfw.KeyLeftShift) == glfw.Press && g.win.GetKey(glfw.KeyW) == glfw.Press {
		g.camera.OnMoveChange(MoveForward, speed*1.2)
	}
	pos := g.camera.Pos()
	stop := false
	if !g.camera.Flying() {
		g.vy -= float32(dt * 20)
		if g.vy < -50 {
			g.vy = -50
		}
		pos = mgl32.Vec3{pos.X(), pos.Y() + g.vy*float32(dt), pos.Z()}
	}

	pos, stop = g.world.CollideStepped(from, pos)
	if stop {
		g.vy = 0
	}
	g.camera.SetPos(pos)
}

func (g *Game) CurrentBlockid() Vec3 {
	pos := g.camera.Pos()
	return NearBlock(pos)
}

func (g *Game) ShouldClose() bool {
	return g.closed
}

func (g *Game) renderStat() {
	g.fps.Update()
	p := g.camera.Pos()
	cid := NearBlock(p).Chunkid()
	stat := g.blockRender.Stat()
	title := fmt.Sprintf("[%.2f %.2f %.2f] %v [%d/%d %d] %d", p.X(), p.Y(), p.Z(),
		cid, stat.RendingChunks, stat.CacheChunks, stat.Faces, g.fps.Fps())
	g.win.SetTitle(title)
}

func (g *Game) syncPlayerLoop() {
	tick := time.NewTicker(time.Second / 10)
	for range tick.C {
		ClientUpdatePlayerState(g.camera.State())
	}
}

func (g *Game) Update() {
	mainthread.Call(func() {
		var dt float64
		now := glfw.GetTime()
		dt = now - g.prevtime
		g.prevtime = now
		if dt > 0.02 {
			dt = 0.02
		}

		g.handleKeyInput(dt)

		// Advance the fire simulation on its own slower cadence.
		if now-g.lastFireTick > fireTick {
			g.fireSim.tick(g.world.Block, g.setSimBlock)
			g.lastFireTick = now
		}

		// Smoke: refresh the emitter set on a throttled cadence, then advance the
		// particle simulation every frame using the cached sources.
		if now-g.lastSmokeScan > smokeScanTick {
			g.refreshSmokeSources()
			g.lastSmokeScan = now
		}
		g.smoke.Step(g.smokeSources, float32(dt))

		// Clear to the horizon sky colour (a fallback under the sky pass) so the
		// horizon matches the time of day.
		sky := gameClock.SkyColor(now)
		gl.ClearColor(sky.X(), sky.Y(), sky.Z(), 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		// Sky (gradient + sun + moon) fills the background before the terrain.
		g.skyRender.Draw(
			g.blockRender.get3dmat().Inv(),
			g.camera.Pos(),
			gameClock.SunDir(now),
			gameClock.Daylight(now),
		)

		g.blockRender.Draw()
		g.lineRender.Draw()
		g.playerRender.Draw()

		// Smoke blends over the terrain and players, so it draws last.
		g.smokeRender.Draw(g.smoke, g.blockRender.get3dmat(), g.smokeScale(), gameClock.Daylight(now))

		g.renderStat()

		g.win.SwapBuffers()
		glfw.PollEvents()
		g.closed = g.win.ShouldClose()
	})
}

type FPS struct {
	lastUpdate time.Time
	cnt        int
	fps        int
}

func (f *FPS) Update() {
	f.cnt++
	now := time.Now()
	p := now.Sub(f.lastUpdate)
	if p >= time.Second {
		f.fps = int(float64(f.cnt) / p.Seconds())
		f.cnt = 0
		f.lastUpdate = now
	}
}

func (f *FPS) Fps() int {
	return f.fps
}

func run() {
	gameClock = NewGameClock(*dayLength)

	err := LoadTextureDesc()
	if err != nil {
		log.Fatal(err)
	}

	err = InitStore()
	if err != nil {
		log.Panic(err)
	}
	defer store.Close()

	err = InitClient()
	if err != nil {
		log.Panic(err)
	}
	if client != nil {
		defer client.Close()
	}

	game, err = NewGame(800, 600)
	if err != nil {
		log.Panic(err)
	}

	game.camera.Restore(store.GetPlayerState())
	game.preload()
	tick := time.Tick(time.Second / 60)
	for !game.ShouldClose() {
		<-tick
		game.Update()
	}
	store.UpdatePlayerState(game.camera.State())
}

// preload builds every chunk within the render radius of the spawn point before
// gameplay starts, showing a progress bar, so the world appears complete on the
// first frame instead of popping in a few chunks at a time.
func (g *Game) preload() {
	needed := neededChunks()
	total := len(needed)
	if total == 0 {
		return
	}
	start := glfw.GetTime()

	// Generate and light every needed chunk up front in one batch, so terrain
	// generation and the parallel light seeding fan out across all cores.
	ids := make([]Vec3, 0, total)
	for id := range needed {
		ids = append(ids, id)
	}
	chunks := g.world.Chunks(ids)

	// Build + upload meshes in parallel batches, showing progress. Face-building
	// (the CPU-heavy part of meshing) runs across cores; only the GL upload is
	// serial on the main thread.
	const batch = 32
	for i := 0; i < len(chunks); i += batch {
		if g.win.ShouldClose() {
			g.closed = true
			return
		}
		end := i + batch
		if end > len(chunks) {
			end = len(chunks)
		}
		sub := chunks[i:end]
		datas := g.blockRender.buildChunksParallel(sub)
		mainthread.Call(func() {
			for j, c := range sub {
				g.blockRender.uploadChunkMesh(c, datas[j])
			}
		})
		g.drawLoading(float64(end) / float64(len(chunks)))
	}

	// Everything is meshed with final light; discard the "needs re-mesh" set the
	// seeding queued so the first frame doesn't re-mesh the whole world.
	g.world.DrainLightDirty()
	log.Printf("preloaded %d chunks in %.1fs", total, glfw.GetTime()-start)
	mainthread.Call(func() { g.win.SetTitle("gocraft") })
}

// drawLoading renders the loading screen: a dark background with a centred
// progress bar (0..1). It uses only glClear + glScissor, so it needs no shader.
func (g *Game) drawLoading(progress float64) {
	mainthread.Call(func() {
		w, h := g.win.GetFramebufferSize()
		gl.Disable(gl.SCISSOR_TEST)
		gl.ClearColor(0.05, 0.06, 0.09, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		barW, barH := w/3, 16
		x, y := (w-barW)/2, h/2-barH/2
		gl.Enable(gl.SCISSOR_TEST)
		// Bar frame.
		gl.Scissor(int32(x-2), int32(y-2), int32(barW+4), int32(barH+4))
		gl.ClearColor(0.24, 0.27, 0.32, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		// Fill proportional to progress.
		gl.Scissor(int32(x), int32(y), int32(float64(barW)*progress), int32(barH))
		gl.ClearColor(0.55, 0.78, 0.55, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		gl.Disable(gl.SCISSOR_TEST)

		g.win.SetTitle(fmt.Sprintf("gocraft - loading %d%%", int(progress*100)))
		g.win.SwapBuffers()
		glfw.PollEvents()
	})
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()
	go func() {
		if *pprofPort != "" {
			log.Fatal(http.ListenAndServe(*pprofPort, nil))
		}
	}()
	mainthread.Run(run)
}
