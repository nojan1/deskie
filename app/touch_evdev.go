package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Linux input event constants
const (
	evSyn    = 0
	evKey    = 1
	evAbs    = 3
	absX     = 0
	absY     = 1
	btnTouch = 0x14a
)

type touchOpts struct {
	swap, invX, invY       bool
	minX, maxX, minY, maxY int
	debug                  bool
}

func findTouchDevice() string {
	d := findByName("/sys/class/input/input*", "ads7846")
	if d == "" {
		return ""
	}
	ev, _ := filepath.Glob(filepath.Join(d, "event*"))
	if len(ev) == 0 {
		return ""
	}
	return "/dev/input/" + filepath.Base(ev[0])
}

func norm(v, lo, hi int) float64 {
	if hi <= lo {
		return 0
	}
	f := float64(v-lo) / float64(hi-lo)
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// mapXY turns raw controller values into screen pixels. Order: swap axes,
// normalize with min/max, then invert.
func (o touchOpts) mapXY(rx, ry, w, h int) (int, int) {
	if o.swap {
		rx, ry = ry, rx
	}
	nx, ny := norm(rx, o.minX, o.maxX), norm(ry, o.minY, o.maxY)
	if o.invX {
		nx = 1 - nx
	}
	if o.invY {
		ny = 1 - ny
	}
	return int(nx * float64(w-1)), int(ny * float64(h-1))
}

func readTouch(path string, w, h int, o touchOpts, out chan<- Event) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "touch:", err)
		return
	}
	defer f.Close()
	buf := make([]byte, 24) // struct input_event on 64-bit
	var rx, ry int
	touching, sent := false, false
	emit := func(k EventKind) {
		x, y := o.mapXY(rx, ry, w, h)
		e := Event{Kind: k, X: x, Y: y}
		if k == TouchMove {
			select {
			case out <- e:
			default: // drop moves if the UI is behind
			}
			return
		}
		out <- e
	}
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			return
		}
		typ := binary.LittleEndian.Uint16(buf[16:])
		code := binary.LittleEndian.Uint16(buf[18:])
		val := int32(binary.LittleEndian.Uint32(buf[20:]))
		switch {
		case typ == evAbs && code == absX:
			rx = int(val)
		case typ == evAbs && code == absY:
			ry = int(val)
		case typ == evKey && code == btnTouch:
			touching = val != 0
			if !touching && sent {
				emit(TouchUp)
				sent = false
			}
		case typ == evSyn && touching:
			if o.debug {
				fmt.Fprintf(os.Stderr, "raw x=%d y=%d\n", rx, ry)
			}
			if !sent {
				sent = true
				emit(TouchDown)
			} else {
				emit(TouchMove)
			}
		}
	}
}
