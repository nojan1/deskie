package widget

import (
	"image"
	"image/color"
)

// dragSlop is how far (px) a touch must move before a List treats it as a
// scroll rather than a tap. Resistive panels jitter; keep this generous.
const dragSlop = 10

// List is a vertically scrolling container. Each item gets the full list
// width and its PreferredSize().Y as height. Dragging scrolls; a short tap
// goes to the item under the finger. Once a drag starts, the item that got
// the Down receives Cancel.
//
// Items are real widgets, so this suits tens to low hundreds of rows. For
// bigger data sets a model-backed variant can recycle row widgets on top of
// the same scrolling logic.
type List struct {
	Base
	group
	Spacing   int
	Indicator color.Color // scroll bar; nil hides it

	offset  int // pixels scrolled from the top
	content int // total content height

	tracking, dragging bool
	startY, startOff   int
}

func NewList(th *Theme) *List { return &List{Indicator: th.TextDim} }

func (l *List) Add(w Widget) {
	l.add(l, w)
	l.layout()
}

func (l *List) Remove(w Widget) {
	if l.remove(w) >= 0 {
		l.layout()
		l.Invalidate()
	}
}

func (l *List) Clear() {
	l.clear()
	l.offset = 0
	l.layout()
	l.Invalidate()
}

// invalidate overrides Base's: children that scrolled out of view must not
// dirty the area around the list.
func (l *List) invalidate(r image.Rectangle) { l.InvalidateRect(r.Intersect(l.bounds)) }

func (l *List) SetBounds(r image.Rectangle) {
	l.Base.SetBounds(r)
	l.layout()
}

func (l *List) layout() {
	y := l.bounds.Min.Y - l.offset
	total := 0
	for i, ch := range l.children {
		if i > 0 {
			y += l.Spacing
			total += l.Spacing
		}
		h := ch.PreferredSize().Y
		ch.SetBounds(image.Rect(l.bounds.Min.X, y, l.bounds.Max.X, y+h))
		y += h
		total += h
	}
	if total != l.content {
		l.Invalidate() // the scroll indicator's size/position depends on it
	}
	l.content = total
	if o := min(l.offset, l.maxOffset()); o != l.offset {
		l.offset = o
		l.layout()
	}
}

func (l *List) maxOffset() int { return max(0, l.content-l.bounds.Dy()) }

// ScrollTo sets the scroll offset (clamped).
func (l *List) ScrollTo(off int) {
	off = max(0, min(off, l.maxOffset()))
	if off == l.offset {
		return
	}
	l.offset = off
	l.layout()
	l.Invalidate()
}

func (l *List) Offset() int { return l.offset }

// MaxOffset is the largest scroll offset (0 when everything fits).
func (l *List) MaxOffset() int { return l.maxOffset() }

func (l *List) PreferredSize() image.Point {
	w := 0
	for _, ch := range l.children {
		w = max(w, ch.PreferredSize().X)
	}
	return image.Pt(w, l.content)
}

func (l *List) Draw(c *Canvas) {
	c.WithClip(l.bounds, func() {
		l.drawChildren(c)
		if l.Indicator != nil && l.maxOffset() > 0 {
			vh := l.bounds.Dy()
			th := max(vh*vh/l.content, 20)
			ty := l.bounds.Min.Y + l.offset*(vh-th)/l.maxOffset()
			r := image.Rect(l.bounds.Max.X-5, ty, l.bounds.Max.X-1, ty+th)
			c.FillRoundRect(r, 2, l.Indicator)
		}
	})
}

func (l *List) HandleEvent(e Event) bool {
	switch e.Kind {
	case Down:
		l.tracking, l.dragging = true, false
		l.startY, l.startOff = e.Pos.Y, l.offset
		l.dispatch(e)
		return true // the list owns the gesture so it can turn it into a scroll
	case Move:
		if !l.tracking {
			return false
		}
		dy := e.Pos.Y - l.startY
		if !l.dragging && (dy > dragSlop || dy < -dragSlop) && l.maxOffset() > 0 {
			l.dragging = true
			l.startY = e.Pos.Y // start scrolling from here, no jump
			l.dispatch(Event{Kind: Cancel, Pos: e.Pos})
			return true
		}
		if l.dragging {
			l.ScrollTo(l.startOff - dy)
			return true
		}
		l.dispatch(e)
		return true
	case Up, Cancel:
		was := l.dragging
		l.tracking, l.dragging = false, false
		if !was {
			l.dispatch(e)
		}
		return true
	}
	return false
}
