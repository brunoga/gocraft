package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestVolumetricInjects(t *testing.T) {
	v := newVolumetricSmoke()
	src := []mgl32.Vec3{{0.5, 10.5, 0.5}}
	v.Step(src)
	if v.count() == 0 {
		t.Fatal("expected smoke density after injecting at a source")
	}
	if v.total() <= 0 {
		t.Fatalf("expected positive total density, got %f", v.total())
	}
}

func TestVolumetricNoSources(t *testing.T) {
	v := newVolumetricSmoke()
	for i := 0; i < 20; i++ {
		v.Step(nil)
	}
	if v.count() != 0 {
		t.Fatalf("expected no smoke without sources, got %d cells", v.count())
	}
}

func TestVolumetricRises(t *testing.T) {
	v := newVolumetricSmoke()
	src := []mgl32.Vec3{{0.5, 10.5, 0.5}}
	// Inject and step a few times; density should appear above the source cell.
	for i := 0; i < 6; i++ {
		v.Step(src)
	}
	above := v.density[Vec3{0, 11, 0}]
	if above <= 0 {
		t.Fatalf("smoke should rise into the cell above the source, got density %f", above)
	}
	// And it should reach higher than one cell up over time.
	if v.density[Vec3{0, 12, 0}] <= 0 {
		t.Errorf("smoke should climb more than one cell; density at +2 is %f", v.density[Vec3{0, 12, 0}])
	}
}

func TestVolumetricDissipates(t *testing.T) {
	v := newVolumetricSmoke()
	src := []mgl32.Vec3{{0.5, 10.5, 0.5}}
	for i := 0; i < 10; i++ {
		v.Step(src)
	}
	if v.count() == 0 {
		t.Fatal("expected smoke before letting it dissipate")
	}
	// With no sources, density decays and prunes away entirely.
	for i := 0; i < 80; i++ {
		v.Step(nil)
	}
	if v.count() != 0 {
		t.Fatalf("expected smoke to fully dissipate, %d cells (total %f) remain", v.count(), v.total())
	}
}

func TestVolumetricRespectsCap(t *testing.T) {
	v := newVolumetricSmoke()
	src := make([]mgl32.Vec3, 5000)
	for i := range src {
		src[i] = mgl32.Vec3{float32(i) + 0.5, 10.5, 0.5}
	}
	for i := 0; i < 10; i++ {
		v.Step(src)
		if v.count() > volMaxCells {
			t.Fatalf("cell count %d exceeded cap %d", v.count(), volMaxCells)
		}
	}
}

func TestVolumetricAppendPoints(t *testing.T) {
	v := newVolumetricSmoke()
	src := []mgl32.Vec3{{0.5, 10.5, 0.5}}
	for i := 0; i < 5; i++ {
		v.Step(src)
	}
	pts := v.appendPoints(nil)
	if len(pts) != v.count() {
		t.Fatalf("expected one point per cell: %d points, %d cells", len(pts), v.count())
	}
	for _, p := range pts {
		if p.alpha <= 0 || p.alpha > 1 {
			t.Errorf("point alpha out of range: %f", p.alpha)
		}
		if p.size != volCellSize {
			t.Errorf("point size %f, want %f", p.size, volCellSize)
		}
	}
}
