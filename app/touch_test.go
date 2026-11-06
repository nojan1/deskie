package main

import (
	"image"
	"testing"
)

// The legacy flags currently deployed must map the measured corner taps
// exactly as before the affine rewrite.
func TestLegacyFlagsAsAffine(t *testing.T) {
	o := touchOpts{swap: true, invX: true, invY: true, minX: 338, maxX: 3761, minY: 518, maxY: 3637}
	a, err := o.transform(480, 320)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ rx, ry, x, y int }{
		{3580, 3760, 0, 6}, {3695, 340, 479, 0}, {485, 3762, 0, 319}, {551, 335, 479, 316}, // rounds; the old code truncated (315)
	} {
		if x, y := a.apply(c.rx, c.ry, 480, 320); x != c.x || y != c.y {
			t.Errorf("raw (%d,%d): got (%d,%d) want (%d,%d)", c.rx, c.ry, x, y, c.x, c.y)
		}
	}
}

// fitAffine recovers a known transform (with swap, mirroring and skew)
// from exact samples, and round-trips through -touch-cal.
func TestFitAffine(t *testing.T) {
	want := affine{0.004, -0.14, 520, -0.1, 0.002, 380}
	targets := calTargets(480, 320)
	// Invert want to generate raw readings for each target.
	det := want[0]*want[4] - want[1]*want[3]
	var raw []image.Point
	for _, p := range targets {
		X, Y := float64(p.X)-want[2], float64(p.Y)-want[5]
		rx := (want[4]*X - want[1]*Y) / det
		ry := (-want[3]*X + want[0]*Y) / det
		raw = append(raw, image.Pt(int(rx+0.5), int(ry+0.5)))
	}
	got, err := fitAffine(raw, targets)
	if err != nil {
		t.Fatal(err)
	}
	back, err := parseAffine(got.String())
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range raw {
		x, y := back.apply(r.X, r.Y, 480, 320)
		if d := (x-targets[i].X)*(x-targets[i].X) + (y-targets[i].Y)*(y-targets[i].Y); d > 2 {
			t.Errorf("target %v: fitted (%d,%d)", targets[i], x, y)
		}
	}
}
