// Desk display UI. One drawing path (image.RGBA), two backends:
//
//	fb    - Pi: RGB565 framebuffer + evdev touch
//	kitty - dev machine: kitty graphics protocol + mouse input (q / Ctrl-C quits)
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"syscall"
	"time"

	"deskui/widget"
)

func toWidgetEvent(e Event) widget.Event {
	k := widget.Move
	switch e.Kind {
	case TouchDown:
		k = widget.Down
	case TouchUp:
		k = widget.Up
	}
	return widget.Event{Kind: k, Pos: image.Pt(e.X, e.Y)}
}

func main() {
	backend := flag.String("backend", "auto", "auto | fb | kitty")
	var t touchOpts
	flag.BoolVar(&t.swap, "touch-swap", false, "swap touch X/Y (applied before min/max)")
	flag.BoolVar(&t.invX, "touch-invx", false, "invert touch X")
	flag.BoolVar(&t.invY, "touch-invy", false, "invert touch Y")
	flag.IntVar(&t.minX, "touch-minx", 0, "raw X at screen left")
	flag.IntVar(&t.maxX, "touch-maxx", 4095, "raw X at screen right")
	flag.IntVar(&t.minY, "touch-miny", 0, "raw Y at screen top")
	flag.IntVar(&t.maxY, "touch-maxy", 4095, "raw Y at screen bottom")
	flag.BoolVar(&t.debug, "touch-debug", false, "print raw touch values to stderr (fb backend)")
	showDirty := flag.Bool("show-dirty", false, "kitty backend: outline the last flushed rect in red")
	showTouches := flag.Bool("show-touches", true, "draw a dot where the screen is touched")
	flag.Parse()

	d, err := openDisplay(*backend, t, *showDirty)
	if err != nil {
		fmt.Fprintln(os.Stderr, "display:", err)
		os.Exit(1)
	}
	defer d.Close()

	w, h := d.Size()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	screen := widget.NewScreen(w, h, newTheme())
	screen.ShowTouches = *showTouches
	buildUI(screen)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case e := <-d.Events():
			if e.Kind == Quit {
				return
			}
			screen.Handle(toWidgetEvent(e))
		case now := <-tick.C:
			screen.Tick(now)
			if r, ok := screen.Render(img); ok {
				if err := d.Flush(img, r); err != nil {
					fmt.Fprintln(os.Stderr, "flush:", err)
					return
				}
			}
		case <-sig:
			return
		}
	}
}

func openDisplay(backend string, t touchOpts, showDirty bool) (Display, error) {
	switch backend {
	case "fb":
		d, err := openFB(t)
		if err != nil {
			return nil, err
		}
		return d, nil
	case "kitty":
		d, err := openKitty(480, 320, showDirty)
		if err != nil {
			return nil, err
		}
		return d, nil
	default: // auto: the Pi's framebuffer if present, else the terminal
		if d, err := openFB(t); err == nil {
			return d, nil
		}
		d, err := openKitty(480, 320, showDirty)
		if err != nil {
			return nil, err
		}
		return d, nil
	}
}
