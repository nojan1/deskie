package widget

import (
	"image"
	"slices"
	"time"
)

// LayerOptions controls how a layer interacts with the layers below it.
type LayerOptions struct {
	// Modal layers receive every Down; nothing below them gets input.
	Modal bool
	// Dim draws Theme.Backdrop over everything below the layer.
	Dim bool
	// DismissOnOutsideTap closes a modal layer when a Down lands outside it.
	DismissOnOutsideTap bool
	// OnClose runs after the layer has been removed, however that happened.
	OnClose func()
}

// Layer is one entry in the Screen's z-order.
type Layer struct {
	s    *Screen
	root Widget
	opts LayerOptions
}

func (l *Layer) Root() Widget { return l.root }

// Close removes the layer. Safe to call from inside an event handler of a
// widget in that layer, and safe to call twice.
func (l *Layer) Close() { l.s.remove(l) }

// Screen owns the layers, routes input and repaints dirty regions.
type Screen struct {
	Theme       *Theme
	ShowTouches bool // debug: draw a dot at the current touch point

	rect     image.Rectangle
	layers   []*Layer
	dirty    image.Rectangle
	captured *Layer
	touch    *image.Point
}

func NewScreen(w, h int, th *Theme) *Screen {
	s := &Screen{Theme: th, rect: image.Rect(0, 0, w, h)}
	s.InvalidateAll()
	return s
}

func (s *Screen) Bounds() image.Rectangle { return s.rect }

func (s *Screen) invalidate(r image.Rectangle) { s.dirty = s.dirty.Union(r.Intersect(s.rect)) }

func (s *Screen) InvalidateAll() { s.invalidate(s.rect) }

// Push adds a layer on top with root placed at r.
func (s *Screen) Push(root Widget, r image.Rectangle, opts LayerOptions) *Layer {
	l := &Layer{s: s, root: root, opts: opts}
	if opts.Modal && s.captured != nil {
		// A modal appearing mid-gesture steals the rest of it.
		c := s.captured
		s.captured = nil
		c.root.HandleEvent(Event{Kind: Cancel})
	}
	attach(s, root)
	root.SetBounds(r)
	s.invalidate(root.Bounds())
	if opts.Dim {
		s.InvalidateAll()
	}
	s.layers = append(s.layers, l)
	return l
}

// SetRoot pushes a full-screen, non-modal base layer.
func (s *Screen) SetRoot(root Widget) *Layer {
	return s.Push(root, s.rect, LayerOptions{})
}

// PushModal centers w on a dimmed backdrop. size of zero uses
// w.PreferredSize(); the result is clamped to the screen.
func (s *Screen) PushModal(w Widget, size image.Point) *Layer {
	if size == (image.Point{}) {
		size = w.PreferredSize()
	}
	size.X = min(size.X, s.rect.Dx())
	size.Y = min(size.Y, s.rect.Dy())
	at := s.rect.Min.Add(s.rect.Size().Sub(size).Div(2))
	return s.Push(w, image.Rectangle{Min: at, Max: at.Add(size)},
		LayerOptions{Modal: true, Dim: true, DismissOnOutsideTap: true})
}

// Top returns the topmost layer, or nil.
func (s *Screen) Top() *Layer {
	if len(s.layers) == 0 {
		return nil
	}
	return s.layers[len(s.layers)-1]
}

func (s *Screen) remove(l *Layer) {
	i := slices.Index(s.layers, l)
	if i < 0 {
		return
	}
	s.layers = slices.Delete(s.layers, i, i+1)
	if s.captured == l {
		s.captured = nil
		l.root.HandleEvent(Event{Kind: Cancel})
	}
	detach(l.root)
	if l.opts.Dim {
		s.InvalidateAll()
	}
	if l.opts.OnClose != nil {
		l.opts.OnClose()
	}
}

// Handle routes one input event. Down goes to the topmost layer that takes
// it (stopping at a modal layer); the rest of the gesture follows it.
func (s *Screen) Handle(e Event) {
	s.trackTouch(e)
	if e.Kind != Down {
		l := s.captured
		if l == nil {
			return
		}
		if e.Kind == Up || e.Kind == Cancel {
			s.captured = nil
		}
		l.root.HandleEvent(e)
		return
	}
	s.captured = nil
	for i := len(s.layers) - 1; i >= 0; i-- {
		l := s.layers[i]
		inside := l.root.Visible() && e.Pos.In(l.root.Bounds())
		if inside && l.root.HandleEvent(e) {
			s.captured = l
			return
		}
		if l.opts.Modal {
			if !inside && l.opts.DismissOnOutsideTap {
				l.Close()
			}
			return
		}
	}
}

func (s *Screen) trackTouch(e Event) {
	if !s.ShowTouches {
		return
	}
	if s.touch != nil {
		s.invalidate(markerRect(*s.touch))
	}
	if e.Kind == Up || e.Kind == Cancel {
		s.touch = nil
		return
	}
	p := e.Pos
	s.touch = &p
	s.invalidate(markerRect(p))
}

func markerRect(p image.Point) image.Rectangle {
	return image.Rect(p.X-10, p.Y-10, p.X+10, p.Y+10)
}

// Tick advances every Ticker in every layer.
func (s *Screen) Tick(now time.Time) {
	for _, l := range slices.Clone(s.layers) {
		Walk(l.root, func(w Widget) {
			if t, ok := w.(Ticker); ok {
				t.Tick(now)
			}
		})
	}
}

// Render repaints the dirty region into img, bottom layer first. It returns
// the region that changed, or ok=false if nothing did.
func (s *Screen) Render(img *image.RGBA) (r image.Rectangle, ok bool) {
	r = s.dirty.Intersect(img.Bounds())
	s.dirty = image.Rectangle{}
	if r.Empty() {
		return r, false
	}
	c := newCanvas(img, r)
	c.FillRect(s.rect, s.Theme.Background)
	for _, l := range s.layers {
		if l.opts.Dim {
			c.FillRect(s.rect, s.Theme.Backdrop)
		}
		if l.root.Visible() && c.Visible(l.root.Bounds()) {
			l.root.Draw(c)
		}
	}
	if s.touch != nil {
		c.SetColor(s.Theme.Touch)
		c.DrawCircle(float64(s.touch.X), float64(s.touch.Y), 8)
		c.Fill()
	}
	return r, true
}
