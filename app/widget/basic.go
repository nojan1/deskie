package widget

import (
	"image"
	"image/color"
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
