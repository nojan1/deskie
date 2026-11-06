package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	// Simple calibration: swap axes, normalize with min/max, then invert.
	swap, invX, invY       bool
	minX, maxX, minY, maxY int
	// cal, if set, overrides the above with an affine transform
	// "a,b,c,d,e,f": x = a*rx + b*ry + c, y = d*rx + e*ry + f (raw -> px).
	// Produce it with -calibrate.
	cal   string
	debug bool
	raw   bool // emit raw controller values (used by -calibrate)
}

// affine maps raw controller values to screen pixels:
// x = a[0]*rx + a[1]*ry + a[2], y = a[3]*rx + a[4]*ry + a[5].
type affine [6]float64

func parseAffine(s string) (affine, error) {
	var a affine
	parts := strings.Split(s, ",")
	if len(parts) != 6 {
		return a, fmt.Errorf("touch-cal: want 6 comma-separated numbers, got %d", len(parts))
	}
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return a, fmt.Errorf("touch-cal: %w", err)
		}
		a[i] = v
	}
	return a, nil
}

func (a affine) String() string {
	return fmt.Sprintf("%.6g,%.6g,%.6g,%.6g,%.6g,%.6g", a[0], a[1], a[2], a[3], a[4], a[5])
}

// transform builds the raw->pixel mapping for a w×h screen.
func (o touchOpts) transform(w, h int) (affine, error) {
	if o.cal != "" {
		return parseAffine(o.cal)
	}
	// The simple flags are an affine map too: one raw axis feeds each
	// screen axis, scaled from [min,max] to [0,size-1], maybe mirrored.
	axis := func(lo, hi, size int, inv bool) (k, c float64) {
		if hi <= lo {
			return 0, 0
		}
		k = float64(size-1) / float64(hi-lo)
		c = -k * float64(lo)
		if inv {
			k, c = -k, float64(size-1)-c
		}
		return k, c
	}
	kx, cx := axis(o.minX, o.maxX, w, o.invX)
	ky, cy := axis(o.minY, o.maxY, h, o.invY)
	if o.swap {
		return affine{0, kx, cx, ky, 0, cy}, nil
	}
	return affine{kx, 0, cx, 0, ky, cy}, nil
}

func (a affine) apply(rx, ry, w, h int) (int, int) {
	fx, fy := float64(rx), float64(ry)
	x := int(a[0]*fx + a[1]*fy + a[2] + 0.5)
	y := int(a[3]*fx + a[4]*fy + a[5] + 0.5)
	return max(0, min(x, w-1)), max(0, min(y, h-1))
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

func readTouch(path string, w, h int, o touchOpts, out chan<- Event) {
	tf, err := o.transform(w, h)
	if err != nil {
		fmt.Fprintln(os.Stderr, "touch:", err)
		return
	}
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
		x, y := rx, ry
		if !o.raw {
			x, y = tf.apply(rx, ry, w, h)
		}
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
