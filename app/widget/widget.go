// Package widget is a small retained-mode widget toolkit for the desk display.
//
// Model:
//   - Every widget embeds Base, which holds its bounds (absolute screen
//     coordinates) and a link to its owner for invalidation.
//   - Containers (Stack, Panel, Window, List) own children and assign their
//     bounds during layout. Widgets never position themselves.
//   - A Screen holds a stack of Layers (base UI, modal overlays, ...). It
//     routes input top-down, tracks dirty regions and repaints only those.
//   - Changing state calls Invalidate(); nothing is drawn until the next
//     Screen.Render, which redraws every layer clipped to the dirty region.
package widget

import (
	"image"
	"time"
)

type EventKind int

const (
	Down EventKind = iota
	Move
	Up
	// Cancel ends a gesture without an Up, e.g. when a List starts scrolling
	// or a modal layer appears. Widgets must drop any pressed state.
	Cancel
)

// Event is a pointer/touch event in screen pixel coordinates.
type Event struct {
	Kind EventKind
	Pos  image.Point
}

// Widget is implemented by embedding Base and overriding what is needed.
// The unexported base method means every widget must embed Base, which keeps
// invalidation and visibility uniform.
type Widget interface {
	Bounds() image.Rectangle
	// SetBounds is called by the owning container during layout.
	SetBounds(r image.Rectangle)
	// PreferredSize is a layout hint; zero components mean "no preference".
	PreferredSize() image.Point
	// Draw paints the widget. The canvas is already clipped to the dirty
	// region; widgets may skip work for parts that are not c.Visible.
	Draw(c *Canvas)
	// HandleEvent returns true if the widget consumed the event. A widget
	// that consumes Down receives the rest of the gesture (Move/Up/Cancel)
	// even when the pointer leaves its bounds.
	HandleEvent(e Event) bool
	Visible() bool
	base() *Base
}

// Container is a widget with children.
type Container interface {
	Widget
	Children() []Widget
}

// Ticker is implemented by widgets that change over time (clocks, animations).
// Screen.Tick calls it on every widget in every layer.
type Ticker interface {
	Tick(now time.Time)
}

// invalidator receives dirty rectangles; implemented by Base (forwarding to
// its own owner), by List (clipping to its viewport) and by Screen (sink).
type invalidator interface {
	invalidate(r image.Rectangle)
}

// Base provides the bookkeeping common to all widgets. Embed it.
type Base struct {
	bounds image.Rectangle
	owner  invalidator
	hidden bool
}

func (b *Base) base() *Base                { return b }
func (b *Base) Bounds() image.Rectangle    { return b.bounds }
func (b *Base) PreferredSize() image.Point { return image.Point{} }
func (b *Base) Draw(*Canvas)               {}
func (b *Base) HandleEvent(Event) bool     { return false }
func (b *Base) Visible() bool              { return !b.hidden }

// SetBounds moves the widget, invalidating both the old and new area.
// Containers override it to re-run their layout after calling this.
func (b *Base) SetBounds(r image.Rectangle) {
	if r == b.bounds {
		return
	}
	b.Invalidate()
	b.bounds = r
	b.Invalidate()
}

// SetVisible hides or shows the widget. Hidden widgets keep their layout slot
// but are neither drawn nor hit-tested.
func (b *Base) SetVisible(v bool) {
	if v == !b.hidden {
		return
	}
	b.hidden = !v
	b.Invalidate()
}

// Invalidate schedules a repaint of the whole widget.
func (b *Base) Invalidate() { b.InvalidateRect(b.bounds) }

// InvalidateRect schedules a repaint of r (screen coordinates).
func (b *Base) InvalidateRect(r image.Rectangle) {
	if b.owner != nil && !r.Empty() {
		b.owner.invalidate(r)
	}
}

func (b *Base) invalidate(r image.Rectangle) { b.InvalidateRect(r) }

func attach(owner invalidator, w Widget) {
	b := w.base()
	if b.owner != nil {
		panic("widget: widget already has an owner")
	}
	b.owner = owner
	owner.invalidate(w.Bounds())
}

func detach(w Widget) {
	b := w.base()
	b.Invalidate()
	b.owner = nil
}

// Walk calls fn for w and, depth-first, all of its descendants.
func Walk(w Widget, fn func(Widget)) {
	fn(w)
	if c, ok := w.(Container); ok {
		for _, ch := range c.Children() {
			Walk(ch, fn)
		}
	}
}
