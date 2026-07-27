package main

import (
	"image"
	_ "image/png"
	"os"
	"testing"
)

// TestEmissiveBlockTilesRender guards against a class of bug where a block's
// atlas tile is invisible in-game. The block fragment shader (block.frag)
// discards texels equal to the magenta key (1,0,1) and samples the atlas
// vertically flipped (1-v), so an itemDesc tile index at row R actually samples
// the tile at row 15-R. A tile index picked from the atlas directly (ignoring
// the flip) can land on a magenta tile and vanish -- which is exactly what
// happened to the torch and fire blocks. This test loads the real atlas and
// asserts the tile each emissive block *actually samples* is opaque, not the
// transparency key.
func TestEmissiveBlockTilesRender(t *testing.T) {
	f, err := os.Open("texture.png")
	if err != nil {
		t.Skipf("texture.png unavailable: %v", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode texture.png: %v", err)
	}
	b := img.Bounds()
	const cols = 16
	tile := b.Dx() / cols

	// sampledTile maps an itemDesc index to the tile the shader samples after its
	// 1-v vertical flip.
	sampledTile := func(idx int) int {
		row, col := idx/cols, idx%cols
		return (15-row)*cols + col
	}
	isMagenta := func(idx int) bool {
		tx, ty := (idx%cols)*tile, (idx/cols)*tile
		r, g, bl, _ := img.At(b.Min.X+tx+tile/2, b.Min.Y+ty+tile/2).RGBA()
		return r>>8 == 255 && g>>8 == 0 && bl>>8 == 255
	}

	for _, tp := range []int{blockTorch, blockFire} {
		desc, ok := itemDesc[tp]
		if !ok {
			t.Errorf("block %d has no itemDesc entry", tp)
			continue
		}
		for face, idx := range desc {
			if isMagenta(sampledTile(idx)) {
				t.Errorf("block %d face %d (tile %d) samples the magenta transparency key and will be invisible; pick a tile whose row-flipped sample is opaque", tp, face, idx)
			}
		}
	}
}
