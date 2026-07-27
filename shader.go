package main

import (
	_ "embed"
	"strings"
)

var (
	//go:embed block.vert
	blockVertexSource string

	//go:embed block.frag
	blockFragmentSource string

	//go:embed line.vert
	lineVertexSource string

	//go:embed line.frag
	lineFragmentSource string

	//go:embed player.vert
	playerVertexSource string

	//go:embed player.frag
	playerFragmentSource string

	//go:embed sky.vert
	skyVertexSource string

	//go:embed sky.frag
	skyFragmentSource string

	//go:embed sky_common.glsl
	skyCommonSource string
)

// withSkyCommon injects the shared sky-colour function (skyBackground) into a
// fragment shader, right after its #version directive, so the sky shader and the
// block fog can share one implementation.
func withSkyCommon(frag string) string {
	if i := strings.IndexByte(frag, '\n'); i >= 0 {
		return frag[:i+1] + "\n" + skyCommonSource + "\n" + frag[i+1:]
	}
	return skyCommonSource + "\n" + frag
}
