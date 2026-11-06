package widget

import (
	"image"
	"image/color"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

// Canvas is a gg.Context plus the current rectangular clip. All gg drawing
// primitives are available; use WithClip rather than gg's Clip/ResetClip so
// the clip stack stays consistent.
type Canvas struct {
	*gg.Context
	clip image.Rectangle
	full image.Rectangle
}

func newCanvas(img *image.RGBA, clip image.Rectangle) *Canvas {
	c := &Canvas{Context: gg.NewContextForRGBA(img), full: img.Bounds(), clip: img.Bounds()}
	c.setClip(clip.Intersect(c.full))
	return c
}

// ClipRect is the area that will actually be painted.
func (c *Canvas) ClipRect() image.Rectangle { return c.clip }

// Visible reports whether anything drawn in r can show up.
func (c *Canvas) Visible(r image.Rectangle) bool { return c.clip.Overlaps(r) }

// WithClip runs fn with the clip narrowed to r. fn is skipped entirely if
// nothing of r is visible.
func (c *Canvas) WithClip(r image.Rectangle, fn func()) {
	old := c.clip
	nr := old.Intersect(r)
	if nr.Empty() {
		return
	}
	if nr == old {
		fn()
		return
	}
	c.setClip(nr)
	fn()
	c.setClip(old)
}

// setClip uses gg's mask. Clipping keeps anti-aliased edges from being
// blended twice when a region is repainted.
func (c *Canvas) setClip(r image.Rectangle) {
	c.clip = r
	c.ResetClip()
	if r != c.full {
		c.DrawRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		c.Clip()
	}
}

func (c *Canvas) FillRect(r image.Rectangle, col color.Color) {
	if col == nil || !c.Visible(r) {
		return
	}
	c.SetColor(col)
	c.DrawRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	c.Fill()
}

func (c *Canvas) FillRoundRect(r image.Rectangle, radius float64, col color.Color) {
	if col == nil || !c.Visible(r) {
		return
	}
	if radius <= 0 {
		c.FillRect(r, col)
		return
	}
	c.SetColor(col)
	c.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), radius)
	c.Fill()
}

// Text draws s inside r. ax/ay anchor both the text and the point in r:
// 0 = left/top, 0.5 = centered, 1 = right/bottom.
func (c *Canvas) Text(s string, face font.Face, col color.Color, r image.Rectangle, ax, ay float64) {
	if s == "" || !c.Visible(r) {
		return
	}
	c.SetFontFace(face)
	c.SetColor(col)
	x := float64(r.Min.X) + ax*float64(r.Dx())
	y := float64(r.Min.Y) + ay*float64(r.Dy())
	c.DrawStringAnchored(s, x, y, ax, ay)
}
