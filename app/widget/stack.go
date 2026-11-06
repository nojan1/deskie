package widget

import (
	"image"
	"image/color"
)

type Axis int

const (
	Vertical Axis = iota
	Horizontal
)

type sizeKind int

const (
	sizeAuto sizeKind = iota
	sizeFixed
	sizeFlex
)

// Size is how much main-axis space a Stack gives a child.
type Size struct {
	kind sizeKind
	v    float64
}

var (
	// Auto uses the child's PreferredSize.
	Auto = Size{kind: sizeAuto}
)

// Fixed gives the child exactly px pixels.
func Fixed(px int) Size { return Size{kind: sizeFixed, v: float64(px)} }

// Flex shares the space left after Fixed and Auto children, by weight.
func Flex(weight float64) Size { return Size{kind: sizeFlex, v: weight} }

type Insets struct{ Top, Right, Bottom, Left int }

func Pad(n int) Insets              { return Insets{n, n, n, n} }
func PadXY(x, y int) Insets         { return Insets{y, x, y, x} }
func (in Insets) Size() image.Point { return image.Pt(in.Left+in.Right, in.Top+in.Bottom) }
func (in Insets) Shrink(r image.Rectangle) image.Rectangle {
	r = image.Rect(r.Min.X+in.Left, r.Min.Y+in.Top, r.Max.X-in.Right, r.Max.Y-in.Bottom)
	if r.Dx() < 0 {
		r.Max.X = r.Min.X
	}
	if r.Dy() < 0 {
		r.Max.Y = r.Min.Y
	}
	return r
}

// Stack lays its children out in a row or column. On the cross axis children
// are stretched to fill. Set fields before the stack is laid out, or call
// Layout afterwards.
type Stack struct {
	Base
	group
	Axis       Axis
	Spacing    int
	Padding    Insets
	Background color.Color // optional
	Radius     float64
	sizes      []Size
}

func NewVStack() *Stack { return &Stack{Axis: Vertical} }
func NewHStack() *Stack { return &Stack{Axis: Horizontal} }

// Add appends a child; returns s for chaining.
func (s *Stack) Add(w Widget, sz Size) *Stack {
	s.add(s, w)
	s.sizes = append(s.sizes, sz)
	s.relayout()
	return s
}

func (s *Stack) Remove(w Widget) {
	if i := s.remove(w); i >= 0 {
		s.sizes = append(s.sizes[:i], s.sizes[i+1:]...)
		s.relayout()
	}
}

func (s *Stack) Clear() {
	s.clear()
	s.sizes = nil
	s.Invalidate()
}

func (s *Stack) relayout() {
	if !s.bounds.Empty() {
		s.Layout()
		s.Invalidate()
	}
}

func (s *Stack) SetBounds(r image.Rectangle) {
	s.Base.SetBounds(r)
	s.Layout()
}

// main/cross helpers turn the problem into "vertical" regardless of Axis.
func (s *Stack) main(p image.Point) int {
	if s.Axis == Horizontal {
		return p.X
	}
	return p.Y
}

func (s *Stack) cross(p image.Point) int {
	if s.Axis == Horizontal {
		return p.Y
	}
	return p.X
}

func (s *Stack) Layout() {
	inner := s.Padding.Shrink(s.bounds)
	n := len(s.children)
	if n == 0 {
		return
	}
	avail := s.main(inner.Size()) - s.Spacing*(n-1)

	lengths := make([]int, n)
	used, totalFlex := 0, 0.0
	for i, ch := range s.children {
		switch sz := s.sizes[i]; sz.kind {
		case sizeFixed:
			lengths[i] = int(sz.v)
		case sizeAuto:
			lengths[i] = s.main(ch.PreferredSize())
		case sizeFlex:
			totalFlex += sz.v
			continue
		}
		used += lengths[i]
	}
	if rest := avail - used; rest > 0 && totalFlex > 0 {
		// Hand out flex space without losing pixels to rounding.
		acc := 0.0
		given := 0
		for i := range s.children {
			if s.sizes[i].kind != sizeFlex {
				continue
			}
			acc += s.sizes[i].v
			end := int(float64(rest)*acc/totalFlex + 0.5)
			lengths[i] = end - given
			given = end
		}
	}

	pos := s.main(inner.Min)
	for i, ch := range s.children {
		var r image.Rectangle
		if s.Axis == Horizontal {
			r = image.Rect(pos, inner.Min.Y, pos+lengths[i], inner.Max.Y)
		} else {
			r = image.Rect(inner.Min.X, pos, inner.Max.X, pos+lengths[i])
		}
		ch.SetBounds(r)
		pos += lengths[i] + s.Spacing
	}
}

func (s *Stack) PreferredSize() image.Point {
	mainSum, crossMax := 0, 0
	for i, ch := range s.children {
		p := ch.PreferredSize()
		m := s.main(p)
		if s.sizes[i].kind == sizeFixed {
			m = int(s.sizes[i].v)
		}
		mainSum += m
		crossMax = max(crossMax, s.cross(p))
	}
	if n := len(s.children); n > 1 {
		mainSum += s.Spacing * (n - 1)
	}
	pad := s.Padding.Size()
	if s.Axis == Horizontal {
		return image.Pt(mainSum, crossMax).Add(pad)
	}
	return image.Pt(crossMax, mainSum).Add(pad)
}

func (s *Stack) Draw(c *Canvas) {
	c.FillRoundRect(s.bounds, s.Radius, s.Background)
	s.drawChildren(c)
}

func (s *Stack) HandleEvent(e Event) bool { return s.dispatch(e) }

// Spacer is an empty widget, typically added with Flex to push siblings apart.
type Spacer struct{ Base }

func NewSpacer() *Spacer { return &Spacer{} }

func (s *Stack) OpaqueRect() image.Rectangle { return opaqueRect(s.bounds, s.Radius, s.Background) }
