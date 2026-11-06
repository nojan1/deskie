package widget

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Canvas is a gg.Context plus clipping. All gg drawing primitives are
// available and take screen coordinates.
//
// Clipping does not use gg's mask (a full-screen alpha image that makes every
// draw cost as much as the whole screen, which is far too slow on a Pi 3).
// Instead each clip region is drawn into a scratch image exactly that size,
// with the context translated so widgets still use screen coordinates. The
// rasterizer's own bounds then do the clipping, so cost scales with the
// area actually repainted. Don't call gg's Clip/ResetClip/Identity; use
// WithClip.
type Canvas struct {
	*gg.Context
	img    *image.RGBA     // current target; pixel (0,0) is screen origin
	origin image.Point     // screen position of img's (0,0)
	clip   image.Rectangle // screen coordinates; == origin + img size
	pool   *scratchPool
}

// newCanvas returns a canvas for repainting clip. Its pixels are
// uninitialized: the caller must paint every pixel (Screen fills the
// background first). Finish with flush.
func newCanvas(pool *scratchPool, clip image.Rectangle) *Canvas {
	img := pool.get(clip.Size(), false)
	return &Canvas{Context: newContext(img, clip.Min), img: img, origin: clip.Min, clip: clip, pool: pool}
}

func newContext(img *image.RGBA, origin image.Point) *gg.Context {
	dc := gg.NewContextForRGBA(img)
	dc.Translate(float64(-origin.X), float64(-origin.Y))
	return dc
}

// flush copies the painted region into dst (screen-sized) and releases the
// scratch buffer.
func (c *Canvas) flush(dst *image.RGBA) {
	draw.Draw(dst, c.clip, c.img, image.Point{}, draw.Src)
	c.pool.put(c.img)
}

// ClipRect is the area that will actually be painted.
func (c *Canvas) ClipRect() image.Rectangle { return c.clip }

// Visible reports whether anything drawn in r can show up.
func (c *Canvas) Visible(r image.Rectangle) bool { return c.clip.Overlaps(r) }

// WithClip runs fn with drawing restricted to r. fn is skipped entirely if
// nothing of r is visible.
func (c *Canvas) WithClip(r image.Rectangle, fn func()) {
	nr := c.clip.Intersect(r)
	if nr.Empty() {
		return
	}
	if nr == c.clip {
		fn()
		return
	}
	// Draw into a transparent layer the size of nr, then composite it over
	// the parent. Over is associative for premultiplied colors, so this is
	// pixel-identical to drawing directly with a clip.
	saved := *c
	layer := c.pool.get(nr.Size(), true)
	c.Context, c.img, c.origin, c.clip = newContext(layer, nr.Min), layer, nr.Min, nr
	fn()
	*c = saved
	draw.Draw(c.img, nr.Sub(c.origin), layer, image.Point{}, draw.Over)
	c.pool.put(layer)
}

// FillRect fills an axis-aligned rectangle. Integer rects need no
// anti-aliasing, so this bypasses the path rasterizer entirely.
func (c *Canvas) FillRect(r image.Rectangle, col color.Color) {
	if col == nil {
		return
	}
	r = r.Intersect(c.clip)
	if r.Empty() {
		return
	}
	op := draw.Over
	if _, _, _, a := col.RGBA(); a == 0xffff {
		op = draw.Src
	}
	draw.Draw(c.img, r.Sub(c.origin), image.NewUniform(col), image.Point{}, op)
}

func (c *Canvas) FillRoundRect(r image.Rectangle, radius float64, col color.Color) {
	if col == nil || !c.Visible(r) {
		return
	}
	rad := min(int(radius+0.5), r.Dx()/2, r.Dy()/2)
	if rad <= 0 {
		c.FillRect(r, col)
		return
	}
	// The straight parts are plain rects (fast blits); only the four corner
	// quarter-discs go through the path rasterizer, which is slow on a Pi.
	// With integer geometry the pieces meet without seams.
	c.FillRect(image.Rect(r.Min.X+rad, r.Min.Y, r.Max.X-rad, r.Max.Y), col)
	c.FillRect(image.Rect(r.Min.X, r.Min.Y+rad, r.Min.X+rad, r.Max.Y-rad), col)
	c.FillRect(image.Rect(r.Max.X-rad, r.Min.Y+rad, r.Max.X, r.Max.Y-rad), col)
	fr := float64(rad)
	corners := []struct {
		sq     image.Rectangle
		cx, cy float64
		a0     float64
	}{
		{image.Rect(r.Min.X, r.Min.Y, r.Min.X+rad, r.Min.Y+rad), float64(r.Min.X + rad), float64(r.Min.Y + rad), math.Pi},
		{image.Rect(r.Max.X-rad, r.Min.Y, r.Max.X, r.Min.Y+rad), float64(r.Max.X - rad), float64(r.Min.Y + rad), 1.5 * math.Pi},
		{image.Rect(r.Max.X-rad, r.Max.Y-rad, r.Max.X, r.Max.Y), float64(r.Max.X - rad), float64(r.Max.Y - rad), 0},
		{image.Rect(r.Min.X, r.Max.Y-rad, r.Min.X+rad, r.Max.Y), float64(r.Min.X + rad), float64(r.Max.Y - rad), 0.5 * math.Pi},
	}
	c.SetColor(col)
	for _, k := range corners {
		if !c.Visible(k.sq) {
			continue
		}
		c.MoveTo(k.cx, k.cy)
		c.DrawArc(k.cx, k.cy, fr, k.a0, k.a0+math.Pi/2)
		c.ClosePath()
		c.Fill()
	}
}

// Text draws s inside r. ax/ay place the text's box (advance width ×
// ascent+descent) within r: 0 = left/top, 0.5 = centered, 1 = right/bottom.
// Using real ascent/descent (rather than gg's anchoring, which ignores the
// descender) keeps glyphs inside r whenever r is tall enough, which dirty-rect
// repainting relies on.
func (c *Canvas) Text(s string, face font.Face, col color.Color, r image.Rectangle, ax, ay float64) {
	if s == "" || !c.Visible(r) {
		return
	}
	m := face.Metrics()
	asc, h := m.Ascent.Ceil(), m.Ascent.Ceil()+m.Descent.Ceil()
	w := font.MeasureString(face, s).Ceil()
	x := r.Min.X + int(ax*float64(r.Dx()-w))
	base := r.Min.Y + int(ay*float64(r.Dy()-h)) + asc
	// Not gg.DrawString: with a translated matrix gg pushes every glyph
	// through a generic bilinear transform, ~10x slower on the Pi. A plain
	// font.Drawer uses the fast masked blit; positions are whole pixels.
	d := font.Drawer{
		Dst:  c.img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x-c.origin.X, base-c.origin.Y),
	}
	d.DrawString(s)
}

// scratchPool recycles scratch images so steady-state rendering doesn't
// churn the garbage collector.
type scratchPool struct{ free [][]uint8 }

func (p *scratchPool) get(size image.Point, zero bool) *image.RGBA {
	n := size.X * size.Y * 4
	var pix []uint8
	for i, b := range p.free {
		if cap(b) >= n {
			pix = b[:n]
			p.free = append(p.free[:i], p.free[i+1:]...)
			break
		}
	}
	if pix == nil {
		pix = make([]uint8, n)
	} else if zero {
		clear(pix)
	}
	return &image.RGBA{Pix: pix, Stride: size.X * 4, Rect: image.Rectangle{Max: size}}
}

func (p *scratchPool) put(img *image.RGBA) {
	if len(p.free) < 4 {
		p.free = append(p.free, img.Pix)
	}
}
