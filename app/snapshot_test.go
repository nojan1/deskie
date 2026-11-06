package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"deskui/widget"
)

// TestSnapshot renders the home screen and the scene picker to PNGs when
// DESKUI_SNAPSHOT_DIR is set; handy for checking layout without a device.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("DESKUI_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("DESKUI_SNAPSHOT_DIR not set")
	}
	s := widget.NewScreen(480, 320, newTheme())
	buildUI(s)
	img := image.NewRGBA(s.Bounds())
	save := func(name string) {
		s.Render(img)
		f, err := os.Create(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, img)
	}
	save("home.png")

	tapAt := func(x, y int) {
		s.Handle(widget.Event{Kind: widget.Down, Pos: image.Pt(x, y)})
		s.Handle(widget.Event{Kind: widget.Up, Pos: image.Pt(x, y)})
	}
	tapAt(80, 240)  // Lamp on
	tapAt(400, 240) // Scene -> modal
	s.Handle(widget.Event{Kind: widget.Down, Pos: image.Pt(240, 200)})
	s.Handle(widget.Event{Kind: widget.Move, Pos: image.Pt(240, 170)})
	s.Handle(widget.Event{Kind: widget.Move, Pos: image.Pt(240, 120)})
	s.Handle(widget.Event{Kind: widget.Up, Pos: image.Pt(240, 120)})
	save("modal.png")
	tapAt(240, 100) // pick a scene
	save("picked.png")
}
