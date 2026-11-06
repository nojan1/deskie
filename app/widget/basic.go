package widget

import (
	"image"
	"image/color"
	"strings"
	"time"

	"golang.org/x/image/font"
)

// Label draws a single line of text. AlignX/AlignY place it inside its
// bounds (0 = left/top, 0.5 = center, 1 = right/bottom).
type Label struct {
	Base
	text           string
	Face           font.Face
	Color          color.Color
	AlignX, AlignY float64
}

func NewLabel(text string, face font.Face, col color.Color) *Label {
	return &Label{text: text, Face: face, Color: col, AlignX: 0.5, AlignY: 0.5}
}

func (l *Label) Text() string { return l.text }

func (l *Label) SetText(s string) {
	if s != l.text {
		l.text = s
		l.Invalidate()
	}
}

func (l *Label) PreferredSize() image.Point { return textSize(l.Face, l.text) }

func (l *Label) Draw(c *Canvas) {
	c.Text(l.text, l.Face, l.Color, l.bounds, l.AlignX, l.AlignY)
}

func textSize(face font.Face, s string) image.Point {
	return image.Pt(font.MeasureString(face, s).Ceil(), face.Metrics().Height.Ceil())
}

// Clock is a Label showing the current time in a time.Format layout. It only
// invalidates when the formatted text actually changes.
type Clock struct {
	Label
	Format string
}

func NewClock(format string, face font.Face, col color.Color) *Clock {
	c := &Clock{Label: *NewLabel(time.Now().Format(format), face, col), Format: format}
	return c
}

func (c *Clock) Tick(now time.Time) { c.SetText(now.Format(c.Format)) }

// Box is a solid rectangle: dividers, color swatches, simple backgrounds.
type Box struct {
	Base
	Color  color.Color
	Radius float64
	Size   image.Point // preferred size
}

func NewBox(col color.Color) *Box { return &Box{Color: col} }

func (b *Box) PreferredSize() image.Point { return b.Size }
func (b *Box) Draw(c *Canvas)             { c.FillRoundRect(b.bounds, b.Radius, b.Color) }

func (b *Box) SetColor(col color.Color) {
	if col != b.Color {
		b.Color = col
		b.Invalidate()
	}
}

// Paragraph is word-wrapped, multi-line text. It wraps to whatever width
// layout gives it; text that doesn't fit vertically is clipped.
type Paragraph struct {
	Base
	text   string
	Face   font.Face
	Color  color.Color
	AlignX float64 // per line: 0 left, 0.5 center, 1 right
}

func NewParagraph(text string, face font.Face, col color.Color) *Paragraph {
	return &Paragraph{text: text, Face: face, Color: col}
}

func (p *Paragraph) SetText(s string) {
	if s != p.text {
		p.text = s
		p.Invalidate()
	}
}

// PreferredSize assumes no wrapping; layout usually decides the width.
func (p *Paragraph) PreferredSize() image.Point {
	lines := strings.Split(p.text, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, font.MeasureString(p.Face, l).Ceil())
	}
	return image.Pt(w, len(lines)*p.Face.Metrics().Height.Ceil())
}

func (p *Paragraph) Draw(c *Canvas) {
	lh := p.Face.Metrics().Height.Ceil()
	r := p.bounds
	c.WithClip(r, func() {
		for i, line := range wrap(p.Face, p.text, r.Dx()) {
			lr := image.Rect(r.Min.X, r.Min.Y+i*lh, r.Max.X, r.Min.Y+(i+1)*lh)
			if lr.Min.Y >= r.Max.Y {
				break
			}
			c.Text(line, p.Face, p.Color, lr, p.AlignX, 0.5)
		}
	})
}

// wrap breaks text into lines no wider than width, at spaces where possible.
func wrap(face font.Face, text string, width int) []string {
	var out []string
	fits := func(s string) bool { return font.MeasureString(face, s).Ceil() <= width }
	for _, para := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			cand := word
			if line != "" {
				cand = line + " " + word
			}
			if fits(cand) {
				line = cand
				continue
			}
			if line != "" {
				out = append(out, line)
			}
			// A single word wider than the line is broken by characters.
			rs := []rune(word)
			for !fits(string(rs)) {
				n := len(rs) - 1
				for n > 1 && !fits(string(rs[:n])) {
					n--
				}
				out = append(out, string(rs[:n]))
				rs = rs[n:]
			}
			line = string(rs)
		}
		out = append(out, line)
	}
	return out
}
