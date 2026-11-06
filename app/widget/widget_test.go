package widget

import (
	"image"
	"testing"

	"golang.org/x/image/font/basicfont"
)

func testTheme() *Theme {
	f := basicfont.Face7x13
	return DefaultTheme(f, f, f)
}

func tap(s *Screen, p image.Point) {
	s.Handle(Event{Kind: Down, Pos: p})
	s.Handle(Event{Kind: Up, Pos: p})
}

func TestStackLayout(t *testing.T) {
	a, b, c := NewSpacer(), NewSpacer(), NewSpacer()
	st := NewVStack()
	st.Padding = Pad(10)
	st.Spacing = 5
	st.Add(a, Fixed(40)).Add(b, Flex(1)).Add(c, Flex(3))
	st.SetBounds(image.Rect(0, 0, 100, 200))

	// inner 80x180, minus 2*5 spacing = 170; fixed 40 => 130 flex: 33 / 97 (rounded)
	want := []image.Rectangle{
		image.Rect(10, 10, 90, 50),
		image.Rect(10, 55, 90, 88),
		image.Rect(10, 93, 90, 190),
	}
	for i, w := range []Widget{a, b, c} {
		if w.Bounds() != want[i] {
			t.Errorf("child %d: got %v want %v", i, w.Bounds(), want[i])
		}
	}
}

func TestButtonTapAndDragOff(t *testing.T) {
	th := testTheme()
	s := NewScreen(200, 100, th)
	clicks := 0
	btn := NewToggle(th, "x", func(*Button) { clicks++ })
	root := NewHStack().Add(btn, Flex(1))
	s.SetRoot(root)

	tap(s, image.Pt(50, 50))
	if clicks != 1 || !btn.On() {
		t.Fatalf("tap: clicks=%d on=%v", clicks, btn.On())
	}
	// Press, slide out, release: no click.
	s.Handle(Event{Kind: Down, Pos: image.Pt(50, 50)})
	s.Handle(Event{Kind: Move, Pos: image.Pt(500, 50)})
	s.Handle(Event{Kind: Up, Pos: image.Pt(500, 50)})
	if clicks != 1 {
		t.Fatalf("drag-off clicked")
	}
}

func TestListScrollCancelsItem(t *testing.T) {
	th := testTheme()
	s := NewScreen(200, 100, th)
	list := NewList(th)
	taps := 0
	var first *ListItem
	for i := 0; i < 10; i++ {
		it := NewListItem(th, "row", func(*ListItem) { taps++ })
		if first == nil {
			first = it
		}
		list.Add(it)
	}
	s.SetRoot(list)

	s.Handle(Event{Kind: Down, Pos: image.Pt(50, 10)})
	if !first.down {
		t.Fatal("item not pressed on down")
	}
	s.Handle(Event{Kind: Move, Pos: image.Pt(50, 40)}) // past slop: becomes a drag
	s.Handle(Event{Kind: Move, Pos: image.Pt(50, 0)})  // scroll down by 40
	s.Handle(Event{Kind: Up, Pos: image.Pt(50, 0)})
	if taps != 0 || first.down {
		t.Fatalf("drag should cancel item: taps=%d down=%v", taps, first.down)
	}
	if list.Offset() != 40 {
		t.Fatalf("offset = %d, want 40", list.Offset())
	}
	tap(s, image.Pt(50, 50))
	if taps != 1 {
		t.Fatalf("tap after scroll: taps=%d", taps)
	}
	list.ScrollTo(1 << 20)
	if list.Offset() != list.maxOffset() {
		t.Fatalf("ScrollTo not clamped: %d", list.Offset())
	}
}

func TestModalBlocksAndDismisses(t *testing.T) {
	th := testTheme()
	s := NewScreen(200, 200, th)
	under := 0
	s.SetRoot(NewVStack().Add(NewButton(th, "u", func(*Button) { under++ }), Flex(1)))

	inside := 0
	closed := false
	win := NewWindow(th, "", NewVStack().Add(NewButton(th, "m", func(*Button) { inside++ }), Flex(1)))
	l := s.PushModal(win, image.Pt(100, 100))
	l.opts.OnClose = func() { closed = true }

	tap(s, image.Pt(100, 100)) // modal center
	if inside != 1 || under != 0 {
		t.Fatalf("inside=%d under=%d", inside, under)
	}
	tap(s, image.Pt(5, 5)) // outside: dismisses, does not reach base layer
	if !closed || under != 0 || s.Top() == l {
		t.Fatalf("closed=%v under=%d", closed, under)
	}
	tap(s, image.Pt(5, 5))
	if under != 1 {
		t.Fatalf("base layer not reachable after dismiss")
	}
}

func TestDirtyTracking(t *testing.T) {
	th := testTheme()
	s := NewScreen(200, 100, th)
	lbl := NewLabel("a", th.Small, th.Text)
	s.SetRoot(NewVStack().Add(NewSpacer(), Fixed(50)).Add(lbl, Flex(1)))
	img := image.NewRGBA(s.Bounds())
	if r, ok := s.Render(img); !ok || r != s.Bounds() {
		t.Fatalf("first render: %v %v", r, ok)
	}
	if _, ok := s.Render(img); ok {
		t.Fatal("nothing changed but render reported dirty")
	}
	lbl.SetText("b")
	if r, ok := s.Render(img); !ok || r != image.Rect(0, 50, 200, 100) {
		t.Fatalf("label dirty rect: %v", r)
	}
}
