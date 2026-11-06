package main

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"deskui/widget"
)

// Touch calibration (-calibrate): show crosshairs at known pixel positions,
// collect raw controller readings, and fit an affine raw->pixel transform by
// least squares (like tslib). Targets are inset from the edges because
// fingers can't reach the outermost pixels past the bezel, which is what
// corner-tap calibration gets wrong. The centre target measures how much
// non-linearity remains after the fit.

const calInset = 30

func calTargets(w, h int) []image.Point {
	return []image.Point{
		{calInset, calInset}, {w - 1 - calInset, calInset},
		{w - 1 - calInset, h - 1 - calInset}, {calInset, h - 1 - calInset},
		{w / 2, h / 2},
	}
}

// calScreen draws the current target and collects raw samples per tap.
type calScreen struct {
	widget.Base
	th      *widget.Theme
	targets []image.Point
	raw     []image.Point // averaged raw reading per completed target
	samples []image.Point
	msg     string
}

func (c *calScreen) done() bool { return len(c.raw) == len(c.targets) }

func (c *calScreen) Draw(cv *widget.Canvas) {
	b := c.Bounds()
	if !c.done() {
		t := c.targets[len(c.raw)]
		col := c.th.Accent
		if len(c.samples) > 0 {
			col = c.th.Touch // pressed
		}
		cv.FillRect(image.Rect(t.X-20, t.Y, t.X+21, t.Y+1), col)
		cv.FillRect(image.Rect(t.X, t.Y-20, t.X+1, t.Y+21), col)
		cv.SetColor(col)
		cv.SetLineWidth(2)
		cv.DrawCircle(float64(t.X)+0.5, float64(t.Y)+0.5, 10)
		cv.Stroke()
	}
	// Instructions in the band between the centre target and the bottom ones.
	tr := image.Rect(b.Min.X+60, b.Min.Y+b.Dy()*5/8, b.Max.X-60, b.Min.Y+b.Dy()*5/8+30)
	msg := c.msg
	if msg == "" {
		msg = fmt.Sprintf("Tap the centre of the cross  %d/%d", len(c.raw)+1, len(c.targets))
	}
	cv.Text(msg, c.th.Small, c.th.Text, tr, 0.5, 0.5)
}

// HandleEvent receives raw controller coordinates in e.Pos.
func (c *calScreen) HandleEvent(e widget.Event) bool {
	if c.done() {
		return true
	}
	switch e.Kind {
	case widget.Down:
		c.samples = append(c.samples[:0], e.Pos)
		c.Invalidate()
	case widget.Move:
		if len(c.samples) > 0 {
			c.samples = append(c.samples, e.Pos)
		}
	case widget.Up:
		s := c.samples
		c.samples = nil
		c.Invalidate()
		// Resistive panels bounce: a 1-2 sample "tap" is noise, and the
		// first/last readings of a press are the least stable.
		if len(s) < 5 {
			return true
		}
		c.raw = append(c.raw, median(s[2:len(s)-2]))
	}
	return true
}

func median(ps []image.Point) image.Point {
	xs, ys := make([]int, len(ps)), make([]int, len(ps))
	for i, p := range ps {
		xs[i], ys[i] = p.X, p.Y
	}
	slices.Sort(xs)
	slices.Sort(ys)
	return image.Pt(xs[len(xs)/2], ys[len(ys)/2])
}

// fitAffine finds the least-squares affine map raw -> screen.
func fitAffine(raw, screen []image.Point) (affine, error) {
	// Normal equations for [rx ry 1]·[a b c] = target, solved per axis.
	var m [3][3]float64
	var bx, by [3]float64
	for i, r := range raw {
		v := [3]float64{float64(r.X), float64(r.Y), 1}
		for j := range 3 {
			for k := range 3 {
				m[j][k] += v[j] * v[k]
			}
			bx[j] += v[j] * float64(screen[i].X)
			by[j] += v[j] * float64(screen[i].Y)
		}
	}
	sx, err := solve3(m, bx)
	if err != nil {
		return affine{}, err
	}
	sy, err := solve3(m, by)
	if err != nil {
		return affine{}, err
	}
	return affine{sx[0], sx[1], sx[2], sy[0], sy[1], sy[2]}, nil
}

func solve3(m [3][3]float64, b [3]float64) ([3]float64, error) {
	det := func(a [3][3]float64) float64 {
		return a[0][0]*(a[1][1]*a[2][2]-a[1][2]*a[2][1]) -
			a[0][1]*(a[1][0]*a[2][2]-a[1][2]*a[2][0]) +
			a[0][2]*(a[1][0]*a[2][1]-a[1][1]*a[2][0])
	}
	d := det(m)
	if math.Abs(d) < 1e-9 {
		return [3]float64{}, errors.New("calibration points are degenerate")
	}
	var out [3]float64
	for col := range 3 { // Cramer's rule
		mc := m
		for row := range 3 {
			mc[row][col] = b[row]
		}
		out[col] = det(mc) / d
	}
	return out, nil
}

// runCalibration takes over the display until all targets are tapped, then
// prints the -touch-cal flag and per-target errors to stdout.
func runCalibration(d Display) error {
	w, h := d.Size()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	s := widget.NewScreen(w, h, newTheme())
	cs := &calScreen{th: s.Theme, targets: calTargets(w, h)}
	s.SetRoot(cs)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()
	var finished time.Time
	for {
		select {
		case e := <-d.Events():
			if e.Kind == Quit {
				return errors.New("calibration aborted")
			}
			// Raw coordinates don't fall inside the screen, so bypass the
			// Screen's hit testing and feed the widget directly.
			cs.HandleEvent(toWidgetEvent(e))
			if cs.done() && finished.IsZero() {
				finished = time.Now()
				a, err := fitAffine(cs.raw, cs.targets)
				if err != nil {
					cs.msg = "Failed: " + err.Error()
					cs.Invalidate()
					break
				}
				worst := 0.0
				fmt.Println("target -> fitted (error px)")
				for i, r := range cs.raw {
					x, y := a.apply(r.X, r.Y, w, h)
					t := cs.targets[i]
					e := math.Hypot(float64(x-t.X), float64(y-t.Y))
					worst = max(worst, e)
					fmt.Printf("  %v raw=%v -> (%d,%d)  %.1f\n", t, r, x, y, e)
				}
				fmt.Printf("-touch-cal=%s\n", a)
				cs.msg = fmt.Sprintf("Done. Largest error %.0f px", worst)
				cs.Invalidate()
			}
		case <-tick.C:
			if r, ok := s.Render(img); ok {
				if err := d.Flush(img, r); err != nil {
					return err
				}
			}
			if !finished.IsZero() && time.Since(finished) > 3*time.Second {
				return nil
			}
		case <-sig:
			return errors.New("calibration aborted")
		}
	}
}
