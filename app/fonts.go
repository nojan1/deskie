package main

import (
	_ "embed"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Static (non-variable) Fira Code files, see fonts/README.txt for where to get them.
// Fira Code is monospaced; no OpenType shaping is done, so its ligatures
// (->, ==, !=) render as separate glyphs, which is fine for a display.

//go:embed fonts/FiraCode-Regular.ttf
var firaRegular []byte

//go:embed fonts/FiraCode-Bold.ttf
var firaBold []byte

// mustFace builds a font face at the given pixel size (DPI 72 => 1pt = 1px).
//
// This uses x/image's opentype rasterizer rather than golang/freetype:
// freetype fills with the even-odd rule, which punches holes where a glyph's
// contours overlap (Fira Code's "1" and "4" do), while x/image/vector
// accumulates coverage so overlaps stay solid.
func mustFace(data []byte, px float64) font.Face {
	f, err := opentype.Parse(data)
	if err != nil {
		panic("font: " + err.Error())
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic("font: " + err.Error())
	}
	return face
}
