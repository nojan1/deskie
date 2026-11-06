package main

import (
	_ "embed"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

// Static (non-variable) Fira Code files, see fonts/README.txt for where to get them.
// Fira Code is monospaced; freetype does no OpenType shaping, so its ligatures
// (->, ==, !=) render as separate glyphs, which is fine for a display.

//go:embed fonts/FiraCode-Regular.ttf
var firaRegular []byte

//go:embed fonts/FiraCode-Bold.ttf
var firaBold []byte

// mustFace builds a font face at the given pixel size (DPI 72 => 1pt = 1px).
func mustFace(data []byte, px float64) font.Face {
	f, err := truetype.Parse(data)
	if err != nil {
		panic("font: " + err.Error())
	}
	return truetype.NewFace(f, &truetype.Options{Size: px, DPI: 72})
}
