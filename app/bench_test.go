package main

import (
	"context"
	"image"
	"testing"
	"time"

	"deskui/widget"
)

// Benchmarks for the costs that matter on the Pi 3. Run on the device with
// the cross-compiled test binary:  ./deskui.test -test.bench . -test.run XXX

func benchApp(b *testing.B) (*widget.Screen, *app, *image.RGBA) {
	s := widget.NewScreen(480, 320, newTheme())
	a := newApp(context.Background(), s, &Config{}, func(fn func()) { fn() })
	img := image.NewRGBA(s.Bounds())
	s.Render(img)
	return s, a, img
}

// Whole screen, e.g. opening/closing a modal.
func BenchmarkRenderFull(b *testing.B) {
	s, _, img := benchApp(b)
	b.ResetTimer()
	for b.Loop() {
		s.InvalidateAll()
		s.Render(img)
	}
}

// The once-a-second case: only the big clock changes.
func BenchmarkRenderClockTick(b *testing.B) {
	s, _, img := benchApp(b)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	b.ResetTimer()
	for b.Loop() {
		now = now.Add(time.Second)
		s.Tick(now)
		r, _ := s.Render(img)
		b.ReportMetric(float64(r.Dx()*r.Dy()), "px/op")
	}
}

// One drag step in the agenda list (list area repaint under a modal).
func BenchmarkRenderListScroll(b *testing.B) {
	s, a, img := benchApp(b)
	a.openAgenda()
	s.Render(img)
	list := findList(s.Top().Root())
	if list == nil {
		b.Fatal("no list")
	}
	for i := 0; i < 40; i++ {
		list.Add(widget.NewListItem(s.Theme, "Some event title", nil))
	}
	s.Render(img)
	b.ResetTimer()
	off := 0
	for b.Loop() {
		off = (off + 7) % list.MaxOffset()
		list.ScrollTo(off)
		s.Render(img)
	}
}

func findList(w widget.Widget) *widget.List {
	var out *widget.List
	widget.Walk(w, func(w widget.Widget) {
		if l, ok := w.(*widget.List); ok {
			out = l
		}
	})
	return out
}

// RGB565 conversion + copy of a full frame into (fake) framebuffer memory.
func BenchmarkFlushRGB565Full(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 480, 320))
	d := &fbDisplay{w: 480, h: 320, stride: 480 * 2, mem: make([]byte, 480*2*320)}
	for b.Loop() {
		d.Flush(img, img.Bounds())
	}
}

// TestPartialRenderMatchesFull checks dirty-rect repaints: after each
// incremental Render, the frame must equal a from-scratch full repaint.
// (±2 per channel: a nested clip composites through a layer, which can
// round differently from drawing directly.)
func TestPartialRenderMatchesFull(t *testing.T) {
	s := widget.NewScreen(480, 320, newTheme())
	s.ShowTouches = true
	a := newApp(context.Background(), s, &Config{Calendar: &CalendarConfig{}}, func(fn func()) { fn() })
	img := image.NewRGBA(s.Bounds())
	ref := image.NewRGBA(s.Bounds())
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	check := func(step string) {
		t.Helper()
		s.Render(img)
		copy(ref.Pix, img.Pix)
		s.InvalidateAll()
		s.Render(ref)
		for i := range img.Pix {
			if d := int(img.Pix[i]) - int(ref.Pix[i]); d > 2 || d < -2 {
				p := i / 4
				t.Fatalf("%s: pixel (%d,%d) differs: partial %v vs full %v", step, p%480, p/480, img.Pix[i&^3:i&^3+4], ref.Pix[i&^3:i&^3+4])
			}
		}
	}
	s.Render(img)
	for i := 0; i < 3; i++ {
		now = now.Add(time.Second)
		s.Tick(now)
		check("clock tick")
	}
	s.Handle(widget.Event{Kind: widget.Down, Pos: image.Pt(100, 250)})
	check("touch down")
	s.Handle(widget.Event{Kind: widget.Up, Pos: image.Pt(100, 250)})
	check("touch up")
	a.openAgenda()
	check("open agenda")
	list := findList(s.Top().Root())
	for i := 0; i < 30; i++ {
		list.Add(widget.NewListItem(s.Theme, "Event number", nil))
	}
	check("fill list")
	for _, off := range []int{13, 57, 200, 3} {
		list.ScrollTo(off)
		check("scroll")
	}
	s.Top().Close()
	check("close agenda")
}
