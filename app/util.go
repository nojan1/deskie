package main

import (
	"image"
	"image/color"
	"image/draw"
)

// fill paints a solid rectangle without anti-aliasing (used for debug overlays).
func fill(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}
