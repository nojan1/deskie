package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type fbDisplay struct {
	f      *os.File
	mem    []byte
	w, h   int
	stride int
	events chan Event
}

func readSys(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

func findByName(glob, want string) string {
	dirs, _ := filepath.Glob(glob)
	for _, d := range dirs {
		if strings.Contains(strings.ToLower(readSys(filepath.Join(d, "name"))), want) {
			return d
		}
	}
	return ""
}

func openFB(t touchOpts) (*fbDisplay, error) {
	d := findByName("/sys/class/graphics/fb*", "ili9486")
	if d == "" {
		return nil, fmt.Errorf("no ili9486 framebuffer found (see /sys/class/graphics/fb*/name)")
	}
	var w, h, stride, bpp int
	fmt.Sscanf(readSys(filepath.Join(d, "virtual_size")), "%d,%d", &w, &h)
	fmt.Sscanf(readSys(filepath.Join(d, "stride")), "%d", &stride)
	fmt.Sscanf(readSys(filepath.Join(d, "bits_per_pixel")), "%d", &bpp)
	if bpp != 16 {
		return nil, fmt.Errorf("expected RGB565 (16 bpp), got %d", bpp)
	}
	f, err := os.OpenFile("/dev/"+filepath.Base(d), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	mem, err := syscall.Mmap(int(f.Fd()), 0, stride*h,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	fb := &fbDisplay{f: f, mem: mem, w: w, h: h, stride: stride, events: make(chan Event, 64)}
	if dev := findTouchDevice(); dev != "" {
		go readTouch(dev, w, h, t, fb.events)
	} else {
		fmt.Fprintln(os.Stderr, "touch: ADS7846 device not found")
	}
	return fb, nil
}

func (d *fbDisplay) Size() (int, int)      { return d.w, d.h }
func (d *fbDisplay) Events() <-chan Event { return d.events }

// Flush converts only the dirty rectangle to RGB565, since SPI bandwidth is
// the bottleneck. (The fbtft driver then flushes changed pages to the panel.)
func (d *fbDisplay) Flush(img *image.RGBA, r image.Rectangle) error {
	r = r.Intersect(img.Bounds()).Intersect(image.Rect(0, 0, d.w, d.h))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		src := img.Pix[img.PixOffset(r.Min.X, y):]
		dst := d.mem[y*d.stride+r.Min.X*2:]
		for x := 0; x < r.Dx(); x++ {
			rr, gg, bb := uint16(src[x*4]), uint16(src[x*4+1]), uint16(src[x*4+2])
			binary.LittleEndian.PutUint16(dst[x*2:], rr>>3<<11|gg>>2<<5|bb>>3)
		}
	}
	return nil
}

func (d *fbDisplay) Close() error {
	syscall.Munmap(d.mem)
	return d.f.Close()
}
