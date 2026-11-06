// Desk display UI. One drawing path (image.RGBA), two backends:
//
//	fb    - Pi: RGB565 framebuffer + evdev touch
//	kitty - dev machine: kitty graphics protocol + mouse input (q / Ctrl-C quits)
package main

import (
	"context"
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
	configPath := flag.String("config", "", "JSON config file (Home Assistant, calendars); see config.go")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Sources run on their own goroutines; they hand UI changes to this loop.
	updates := make(chan func(), 256)
	post := func(fn func()) {
		select {
		case updates <- fn:
		case <-ctx.Done():
		}
	}
	a := newApp(ctx, screen, cfg, post)
	a.startSources()

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
		case fn := <-updates:
			fn()
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
		// At boot the panel's driver may not have probed yet: wait for it
		// rather than exiting into systemd's restart rate limit.
		deadline := time.Now().Add(30 * time.Second)
		for {
			d, err := openFB(t)
			if err == nil {
				return d, nil
			}
			if time.Now().After(deadline) {
				return nil, err
			}
			time.Sleep(time.Second)
		}
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
