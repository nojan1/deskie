package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"deskui/source/hass"
	"deskui/widget"
)

func TestFormatState(t *testing.T) {
	for _, c := range []struct{ state, unit, want string }{
		{"21.53", "°C", "21.5°C"},
		{"21.0", "°C", "21°C"},
		{"41", "%", "41%"},
		{"1234.6", "W", "1235 W"},
		{"0.26", "kWh", "0.3 kWh"},
		{"running", "", "Running"},
		{"not_home", "", "Not home"},
		{"unavailable", "°C", "–"},
	} {
		if got := formatState(c.state, c.unit); got != c.want {
			t.Errorf("formatState(%q, %q) = %q, want %q", c.state, c.unit, got, c.want)
		}
	}
}

func TestTodayHighLow(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	now := time.Date(2026, 11, 7, 15, 0, 0, 0, loc)

	daily := `{"weather.home": {"forecast": [
		{"datetime": "2026-11-07T11:00:00+00:00", "temperature": 9.6, "templow": 2.1},
		{"datetime": "2026-11-08T11:00:00+00:00", "temperature": 7, "templow": 1}]}}`
	if hi, lo, ok := todayHighLow(json.RawMessage(daily), "weather.home", now); !ok || hi != 9.6 || lo != 2.1 {
		t.Errorf("daily: %v %v %v", hi, lo, ok)
	}

	hourly := `{"weather.home": {"forecast": [
		{"datetime": "2026-11-07T15:00:00+00:00", "temperature": 8},
		{"datetime": "2026-11-07T20:00:00+00:00", "temperature": 4},
		{"datetime": "2026-11-07T23:00:00+00:00", "temperature": -5}]}}`
	// The last entry is already tomorrow in CET.
	if hi, lo, ok := todayHighLow(json.RawMessage(hourly), "weather.home", now); !ok || hi != 8 || lo != 4 {
		t.Errorf("hourly: %v %v %v", hi, lo, ok)
	}

	if _, _, ok := todayHighLow(json.RawMessage(`{}`), "weather.home", now); ok {
		t.Error("empty response should not give a forecast")
	}
}

func TestNightConfig(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, `{"night": {"start": "22:30", "end": "06:45"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n := cfg.Night; n.startMin != 22*60+30 || n.endMin != 6*60+45 || n.WakeSeconds != 60 {
		t.Fatalf("night: %+v", n)
	}
	for _, bad := range []string{
		`{"night": {"start": "22:30"}}`,
		`{"night": {"start": "25:00", "end": "06:00"}}`,
		`{"night": {"start": "late", "end": "06:00"}}`,
	} {
		if _, err := loadConfig(writeConfig(t, bad)); err == nil || !strings.Contains(err.Error(), "night") {
			t.Errorf("%s: want a night error, got %v", bad, err)
		}
	}
}

func TestNightMode(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, `{"night": {"start": "23:00", "end": "07:00", "away_entity": "zone.home", "wake_seconds": 30}}`))
	if err != nil {
		t.Fatal(err)
	}
	s := widget.NewScreen(480, 320, newTheme())
	n := newNightMode(s, cfg.Night)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 7, h, m, 0, 0, time.Local) }

	n.Tick(at(12, 0))
	if n.active() {
		t.Fatal("night mode during the day")
	}
	n.Tick(at(23, 0))
	if !n.active() {
		t.Fatal("no night mode at 23:00")
	}
	// A tap wakes it for wake_seconds; touches while awake extend that.
	n.wake(at(23, 10))
	if n.active() {
		t.Fatal("tap did not wake")
	}
	n.Touched(at(23, 10).Add(20 * time.Second))
	n.Tick(at(23, 10).Add(40 * time.Second))
	if n.active() {
		t.Fatal("touch did not extend the wake period")
	}
	n.Tick(at(23, 11))
	if !n.active() {
		t.Fatal("did not return to night mode after waking")
	}
	n.Tick(at(7, 0))
	if n.active() {
		t.Fatal("still night mode at 07:00")
	}

	// The rest happens the next day, well after the wake period above.
	at = func(h, m int) time.Time { return time.Date(2026, 11, 8, h, m, 0, 0, time.Local) }

	// Away (zone.home with nobody in it) also means night.
	n.setAwayState(hass.State{EntityID: "zone.home", State: "0"})
	n.Tick(at(12, 0))
	if !n.active() {
		t.Fatal("no night mode while away")
	}
	n.wake(at(12, 0))
	n.setAwayState(hass.State{EntityID: "zone.home", State: "1"})
	n.Tick(at(12, 5))
	if n.active() {
		t.Fatal("night mode with someone home")
	}

	// Manual night mode stays until tapped, then doesn't come back.
	n.Enter(at(12, 10))
	n.Tick(at(12, 30))
	if !n.active() {
		t.Fatal("manual night mode ended by itself")
	}
	n.wake(at(12, 31))
	n.Tick(at(12, 45))
	if n.active() {
		t.Fatal("manual night mode came back after waking")
	}
}

func TestNightViewTapWakes(t *testing.T) {
	s := widget.NewScreen(480, 320, newTheme())
	n := newNightMode(s, nil)
	n.Enter(time.Now())
	s.Handle(widget.Event{Kind: widget.Down, Pos: s.Bounds().Min.Add(s.Bounds().Size().Div(2))})
	if !n.active() {
		t.Fatal("woke on touch down; should wait for the finger to lift")
	}
	s.Handle(widget.Event{Kind: widget.Up, Pos: s.Bounds().Min})
	if n.active() {
		t.Fatal("tap did not wake")
	}
}

func TestBooleanHelperButton(t *testing.T) {
	cfg := &Config{HomeAssistant: &HAConfig{URL: "http://x", Buttons: []HAEntity{
		{"", "input_boolean.guest_mode"},
		{"Coffee", "input_boolean.coffee"},
	}}}
	s := widget.NewScreen(480, 320, newTheme())
	a := newApp(context.Background(), s, cfg, func(fn func()) { fn() })

	guest := a.haButtons["input_boolean.guest_mode"]
	if guest == nil || guest.Label() != "Guest mode" {
		t.Fatalf("placeholder label: %v", guest)
	}
	a.setHAState(hass.State{EntityID: "input_boolean.guest_mode", State: "on",
		Attributes: map[string]any{"friendly_name": "Guests"}})
	if !guest.On() || guest.Label() != "Guests" {
		t.Fatalf("helper button: on=%v label=%q", guest.On(), guest.Label())
	}
	a.setHAState(hass.State{EntityID: "input_boolean.coffee", State: "off",
		Attributes: map[string]any{"friendly_name": "Coffee machine"}})
	if b := a.haButtons["input_boolean.coffee"]; b.On() || b.Label() != "Coffee" {
		t.Fatalf("configured label replaced: %q", b.Label())
	}
}
