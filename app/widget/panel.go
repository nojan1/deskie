package widget

import (
	"image"
	"image/color"
)

// Panel places children at fixed rectangles relative to its own top-left
// corner. Useful for hand-tuned screens; prefer Stack when things should
// adapt to size.
type Panel struct {
	Base
	group
	Background color.Color // optional; an opaque panel swallows taps
	Radius     float64
	rel        []image.Rectangle
}

func NewPanel() *Panel { return &Panel{} }

// Place adds w at r (panel-relative). Returns p for chaining.
func (p *Panel) Place(w Widget, r image.Rectangle) *Panel {
	p.add(p, w)
	p.rel = append(p.rel, r)
	w.SetBounds(r.Add(p.bounds.Min))
	return p
}

// Move changes the panel-relative rectangle of an existing child.
func (p *Panel) Move(w Widget, r image.Rectangle) {
	for i, ch := range p.children {
		if ch == w {
			p.rel[i] = r
			w.SetBounds(r.Add(p.bounds.Min))
			return
		}
	}
}

func (p *Panel) Remove(w Widget) {
	if i := p.remove(w); i >= 0 {
		p.rel = append(p.rel[:i], p.rel[i+1:]...)
	}
}

func (p *Panel) SetBounds(r image.Rectangle) {
	p.Base.SetBounds(r)
	for i, ch := range p.children {
		ch.SetBounds(p.rel[i].Add(r.Min))
	}
}

func (p *Panel) PreferredSize() image.Point {
	var u image.Rectangle
	for _, r := range p.rel {
		u = u.Union(r)
	}
	return u.Max
}

func (p *Panel) Draw(c *Canvas) {
	c.FillRoundRect(p.bounds, p.Radius, p.Background)
	p.drawChildren(c)
}

func (p *Panel) HandleEvent(e Event) bool {
	if p.dispatch(e) {
		return true
	}
	return e.Kind == Down && p.Background != nil
}

func (p *Panel) OpaqueRect() image.Rectangle { return opaqueRect(p.bounds, p.Radius, p.Background) }
