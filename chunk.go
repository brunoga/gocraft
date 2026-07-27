package main

import (
	"log"
	"math"
	"sync"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	ChunkWidth = 32
)

type Vec3 struct {
	X, Y, Z int
}

func (v Vec3) Left() Vec3 {
	return Vec3{v.X - 1, v.Y, v.Z}
}
func (v Vec3) Right() Vec3 {
	return Vec3{v.X + 1, v.Y, v.Z}
}
func (v Vec3) Up() Vec3 {
	return Vec3{v.X, v.Y + 1, v.Z}
}
func (v Vec3) Down() Vec3 {
	return Vec3{v.X, v.Y - 1, v.Z}
}
func (v Vec3) Front() Vec3 {
	return Vec3{v.X, v.Y, v.Z + 1}
}
func (v Vec3) Back() Vec3 {
	return Vec3{v.X, v.Y, v.Z - 1}
}
func (v Vec3) Chunkid() Vec3 {
	return Vec3{
		int(math.Floor(float64(v.X) / ChunkWidth)),
		0,
		int(math.Floor(float64(v.Z) / ChunkWidth)),
	}
}

func NearBlock(pos mgl32.Vec3) Vec3 {
	return Vec3{
		int(round(pos.X())),
		int(round(pos.Y())),
		int(round(pos.Z())),
	}
}

type Chunk struct {
	id     Vec3
	blocks sync.Map // map[Vec3]int

	// light holds per-voxel light levels (0..MaxLight) for this chunk's column,
	// indexed by lightIndex. Allocated lazily on first write. Access is
	// serialized by World.lightMu, not by the sync.Map above.
	light []uint8

	// maxY tracks the highest block in this chunk, so light seeding can skip the
	// empty sky above it instead of walking the full world height every column.
	maxY int
}

func NewChunk(id Vec3) *Chunk {
	c := &Chunk{
		id: id,
	}
	return c
}

func (c *Chunk) Id() Vec3 {
	return c.id
}

func (c *Chunk) Block(id Vec3) int {
	if id.Chunkid() != c.id {
		log.Panicf("id %v chunk %v", id, c.id)
	}
	w, ok := c.blocks.Load(id)
	if ok {
		return w.(int)
	}
	return 0
}

func (c *Chunk) add(id Vec3, w int) {
	if id.Chunkid() != c.id {
		log.Panicf("id %v chunk %v", id, c.id)
	}
	c.blocks.Store(id, w)
	if id.Y > c.maxY {
		c.maxY = id.Y
	}
}

func (c *Chunk) del(id Vec3) {
	if id.Chunkid() != c.id {
		log.Panicf("id %v chunk %v", id, c.id)
	}
	c.blocks.Delete(id)
}

func (c *Chunk) RangeBlocks(f func(id Vec3, w int)) {
	c.blocks.Range(func(key, value interface{}) bool {
		f(key.(Vec3), value.(int))
		return true
	})
}

// lightIndex maps an absolute block position within this chunk's column to an
// index into the light slice.
func (c *Chunk) lightIndex(id Vec3) int {
	lx := id.X - c.id.X*ChunkWidth
	lz := id.Z - c.id.Z*ChunkWidth
	return (id.Y*ChunkWidth+lx)*ChunkWidth + lz
}

// getLight returns the stored light level for id (0 if never lit). Callers must
// hold World.lightMu and pass an id within this chunk and 0 <= Y < WorldHeight.
func (c *Chunk) getLight(id Vec3) uint8 {
	if c.light == nil {
		return 0
	}
	return c.light[c.lightIndex(id)]
}

// setLight stores the light level for id, allocating the light slice on first
// use. Callers must hold World.lightMu and pass an id within this chunk and
// 0 <= Y < WorldHeight.
func (c *Chunk) setLight(id Vec3, v uint8) {
	if c.light == nil {
		c.light = make([]uint8, ChunkWidth*ChunkWidth*WorldHeight)
	}
	c.light[c.lightIndex(id)] = v
}
