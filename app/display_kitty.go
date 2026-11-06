package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
)

// kittyDisplay renders the frame as an image via the kitty graphics protocol
// and turns mouse input (pixel-precise SGR reporting) into touch events.
// Press q or Ctrl-C to quit.
type kittyDisplay struct {
	out       *bufio.Writer
	w, h      int
	events    chan Event
	restore   func()
	once      sync.Once
	shadow    *image.RGBA // what the "panel" currently holds; only dirty rects update it
	showDirty bool
	warnings  []string
	warned    map[string]bool
}

func openKitty(w, h int, showDirty bool) (*kittyDisplay, error) {
	restore, err := makeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("terminal raw mode: %w", err)
	}
	k := &kittyDisplay{
		out: bufio.NewWriterSize(os.Stdout, 64<<10), w: w, h: h,
		events: make(chan Event, 256), restore: restore,
		shadow: image.NewRGBA(image.Rect(0, 0, w, h)), showDirty: showDirty,
		warned: map[string]bool{},
	}
	// Start with a loud checkerboard: anything the UI never flushes stays visible.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{0xff, 0x00, 0xff, 0xff}
			if (x/8+y/8)&1 == 1 {
				c = color.RGBA{0x30, 0x00, 0x30, 0xff}
			}
			k.shadow.SetRGBA(x, y, c)
		}
	}
	// alt screen, hide cursor, clear, mouse: button+drag (1002), SGR (1006), SGR pixels (1016)
	fmt.Fprint(k.out, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[?1002h\x1b[?1006h\x1b[?1016h")
	k.out.Flush()
	go k.readInput()
	return k, nil
}

func (k *kittyDisplay) Size() (int, int)      { return k.w, k.h }
func (k *kittyDisplay) Events() <-chan Event { return k.events }

func (k *kittyDisplay) warn(format string, args ...any) {
	m := fmt.Sprintf(format, args...)
	if !k.warned[m] {
		k.warned[m] = true
		k.warnings = append(k.warnings, m)
	}
}

// Flush mimics the SPI panel: only r is copied into the shadow "panel memory",
// so a dirty rect that is too small leaves stale pixels on screen, exactly as
// on the Pi. (The terminal still receives the whole shadow image; what changes
// visibly is limited to r.) Suspicious rects are collected as warnings and
// printed when the program exits.
func (k *kittyDisplay) Flush(img *image.RGBA, r image.Rectangle) error {
	if img.Bounds() != k.shadow.Bounds() {
		k.warn("frame is %v but display is %v", img.Bounds(), k.shadow.Bounds())
	}
	clipped := r.Intersect(k.shadow.Bounds()).Intersect(img.Bounds())
	if r.Empty() {
		k.warn("empty flush rect %v", r)
	} else if clipped != r {
		k.warn("flush rect %v exceeds display, clipped to %v", r, clipped)
	}
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		so, do, n := img.PixOffset(clipped.Min.X, y), k.shadow.PixOffset(clipped.Min.X, y), clipped.Dx()*4
		copy(k.shadow.Pix[do:do+n], img.Pix[so:so+n])
	}
	frame := k.shadow
	if k.showDirty && !clipped.Empty() {
		frame = image.NewRGBA(k.shadow.Bounds())
		copy(frame.Pix, k.shadow.Pix)
		red := color.RGBA{0xff, 0x20, 0x20, 0xff}
		fill(frame, image.Rect(clipped.Min.X, clipped.Min.Y, clipped.Max.X, clipped.Min.Y+1), red)
		fill(frame, image.Rect(clipped.Min.X, clipped.Max.Y-1, clipped.Max.X, clipped.Max.Y), red)
		fill(frame, image.Rect(clipped.Min.X, clipped.Min.Y, clipped.Min.X+1, clipped.Max.Y), red)
		fill(frame, image.Rect(clipped.Max.X-1, clipped.Min.Y, clipped.Max.X, clipped.Max.Y), red)
	}
	return k.send(frame)
}

// send transmits a full frame (zlib-compressed RGBA). Re-sending image id 1 /
// placement id 1 replaces the picture in place at the top-left corner.
func (k *kittyDisplay) send(img *image.RGBA) error {
	var zbuf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&zbuf, zlib.BestSpeed)
	zw.Write(img.Pix)
	zw.Close()
	enc := base64.StdEncoding.EncodeToString(zbuf.Bytes())

	k.out.WriteString("\x1b[H")
	const chunk = 4096
	for first := true; len(enc) > 0; first = false {
		n := min(chunk, len(enc))
		more := 0
		if n < len(enc) {
			more = 1
		}
		if first {
			fmt.Fprintf(k.out, "\x1b_Ga=T,f=32,o=z,s=%d,v=%d,i=1,p=1,q=2,C=1,m=%d;", k.w, k.h, more)
		} else {
			fmt.Fprintf(k.out, "\x1b_Gq=2,m=%d;", more)
		}
		k.out.WriteString(enc[:n])
		k.out.WriteString("\x1b\\")
		enc = enc[n:]
	}
	return k.out.Flush()
}

func (k *kittyDisplay) readInput() {
	r := bufio.NewReader(os.Stdin)
	down := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		switch b {
		case 3, 'q': // Ctrl-C, q
			k.events <- Event{Kind: Quit}
			continue
		case 0x1b:
		default:
			continue
		}
		if c, err := r.ReadByte(); err != nil || c != '[' {
			continue
		}
		var params []byte
		var final byte
		for {
			c, err := r.ReadByte()
			if err != nil {
				return
			}
			if c >= 0x40 && c <= 0x7e {
				final = c
				break
			}
			params = append(params, c)
		}
		if len(params) < 2 || params[0] != '<' || (final != 'M' && final != 'm') {
			continue
		}
		var bn, x, y int
		if _, err := fmt.Sscanf(string(params[1:]), "%d;%d;%d", &bn, &x, &y); err != nil {
			continue
		}
		if bn&64 != 0 || bn&3 != 0 { // wheel or non-left button
			continue
		}
		x, y = max(x-1, 0), max(y-1, 0) // reports are 1-based
		motion := bn&32 != 0
		switch {
		case final == 'm':
			if down {
				down = false
				k.events <- Event{TouchUp, x, y}
			}
		case motion:
			if down {
				select {
				case k.events <- Event{TouchMove, x, y}:
				default:
				}
			}
		default:
			down = true
			k.events <- Event{TouchDown, x, y}
		}
	}
}

func (k *kittyDisplay) Close() error {
	k.once.Do(func() {
		// delete the image, disable mouse modes, show cursor, leave alt screen
		fmt.Fprint(k.out, "\x1b_Ga=d,d=I,i=1,q=2\x1b\\\x1b[?1016l\x1b[?1006l\x1b[?1002l\x1b[?25h\x1b[?1049l")
		k.out.Flush()
		k.restore()
		for _, w := range k.warnings {
			fmt.Fprintln(os.Stderr, "kitty display warning:", w)
		}
	})
	return nil
}
