package main

import (
	"time"

	"deskui/source/hass"
	"deskui/widget"
)

// nightMode decides when the screen shows the night view: a black screen
// with a dim clock on top of everything else. The panel's backlight is
// always on, so black is as dark as it gets.
//
// It is on when entered by hand (Night button), or automatically during the
// configured hours or while nobody is home. A tap leaves manual night mode
// for good, and automatic night mode for cfg.WakeSeconds; touching the
// screen while awake keeps it awake.
type nightMode struct {
	s     *widget.Screen
	th    *widget.Theme
	cfg   NightConfig // zero value: no schedule, no away entity
	layer *widget.Layer

	manual    bool
	away      bool
	wakeUntil time.Time
}

func newNightMode(s *widget.Screen, cfg *NightConfig) *nightMode {
	n := &nightMode{s: s, th: s.Theme}
	if cfg != nil {
		n.cfg = *cfg
	} else {
		n.cfg.startMin, n.cfg.endMin = -1, -1
	}
	if n.cfg.WakeSeconds <= 0 {
		n.cfg.WakeSeconds = 60
	}
	return n
}

func (n *nightMode) active() bool { return n.layer != nil }

// scheduled reports whether now is inside the configured night hours.
func (n *nightMode) scheduled(now time.Time) bool {
	start, end := n.cfg.startMin, n.cfg.endMin
	if start < 0 || start == end {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end // wraps past midnight
}

func (n *nightMode) auto(now time.Time) bool { return n.scheduled(now) || n.away }

// Tick shows or hides the night view as the conditions change.
func (n *nightMode) Tick(now time.Time) {
	want := n.manual || (n.auto(now) && !now.Before(n.wakeUntil))
	switch {
	case want && n.layer == nil:
		n.enter()
	case !want && n.layer != nil:
		n.layer.Close()
	}
}

// Enter is the Night button.
func (n *nightMode) Enter(now time.Time) {
	n.manual = true
	n.Tick(now)
}

// wake is a tap on the night view.
func (n *nightMode) wake(now time.Time) {
	n.manual = false
	n.wakeUntil = now.Add(time.Duration(n.cfg.WakeSeconds) * time.Second)
	n.Tick(now)
}

// Touched is called for every touch while awake; it postpones the return to
// automatic night mode.
func (n *nightMode) Touched(now time.Time) {
	if n.layer == nil && n.wakeUntil.After(now) {
		n.wakeUntil = now.Add(time.Duration(n.cfg.WakeSeconds) * time.Second)
	}
}

func (n *nightMode) setAwayState(s hass.State) {
	switch s.State {
	case "0", "not_home", "away", "off":
		n.away = true
	default:
		n.away = false
	}
}

func (n *nightMode) enter() {
	v := &nightView{onWake: func() { n.wake(time.Now()) }}
	v.Stack = *widget.NewVStack()
	v.Background = n.th.Night
	v.Add(widget.NewClock("15:04", n.th.Large, n.th.NightText), widget.Flex(1))
	// Modal so nothing underneath gets the tap; not dimmed, it is opaque.
	n.layer = n.s.Push(v, n.s.Bounds(), widget.LayerOptions{
		Modal:   true,
		OnClose: func() { n.layer = nil },
	})
}

// nightView is the full-screen night layer. It swallows every touch and
// wakes the screen when the finger lifts.
type nightView struct {
	widget.Stack
	onWake func()
}

func (v *nightView) HandleEvent(e widget.Event) bool {
	if e.Kind == widget.Up {
		v.onWake()
	}
	return true
}

// Opaque, so the screen skips painting the layers underneath.
var _ widget.Occluder = (*nightView)(nil)
