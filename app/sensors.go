package main

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"deskui/source/hass"
	"deskui/widget"
)

// sensorTile is a read-only tile: a small label over a Home Assistant
// entity's state.
type sensorTile struct {
	widget.Base
	th           *widget.Theme
	label, value string
	fixedLabel   bool // label came from config; don't replace with friendly_name
}

func newSensorTile(th *widget.Theme, e HAEntity) *sensorTile {
	t := &sensorTile{th: th, label: e.Label, value: "–", fixedLabel: e.Label != ""}
	if t.label == "" {
		t.label = e.Entity
	}
	return t
}

func (t *sensorTile) set(label, value string) {
	if label != t.label || value != t.value {
		t.label, t.value = label, value
		t.Invalidate()
	}
}

func (t *sensorTile) Draw(c *widget.Canvas) {
	r := t.Bounds()
	c.FillRoundRect(r, 10, t.th.Surface)
	in := widget.PadXY(10, 3).Shrink(r)
	mid := in.Min.Y + in.Dy()*2/5
	c.WithClip(in, func() {
		c.Text(t.label, t.th.Small, t.th.TextDim, image.Rect(in.Min.X, in.Min.Y, in.Max.X, mid), 0, 0.5)
		c.Text(t.value, t.th.Medium, t.th.Text, image.Rect(in.Min.X, mid, in.Max.X, in.Max.Y), 0, 0.5)
	})
}

func (t *sensorTile) setState(s hass.State) {
	label := t.label
	if !t.fixedLabel {
		label = s.Name()
	}
	unit, _ := s.Attributes["unit_of_measurement"].(string)
	t.set(label, formatState(s.State, unit))
}

// formatState renders a sensor state for a tile: numbers rounded to at most
// one decimal with their unit, other states capitalized.
func formatState(state, unit string) string {
	switch state {
	case "unavailable", "unknown", "":
		return "–"
	}
	v, err := strconv.ParseFloat(state, 64)
	if err != nil {
		rs := []rune(strings.ReplaceAll(state, "_", " "))
		return strings.ToUpper(string(rs[:1])) + string(rs[1:])
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if v == math.Trunc(v) || math.Abs(v) >= 100 {
		s = strconv.FormatFloat(v, 'f', 0, 64)
	}
	switch {
	case unit == "":
		return s
	case unit == "%" || strings.HasPrefix(unit, "°"):
		return s + unit
	default:
		return fmt.Sprintf("%s %s", s, unit)
	}
}
