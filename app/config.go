package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"deskui/source/calendar"
)

// Config is read from the -config JSON file. Secret fields (token, calendar
// URLs) may be written as "$VAR" / "${VAR}" and are taken from the
// environment, so the file itself can live in the Nix store while the
// secrets come from a systemd EnvironmentFile.
type Config struct {
	HomeAssistant *HAConfig       `json:"home_assistant"`
	Calendar      *CalendarConfig `json:"calendar"`
	Night         *NightConfig    `json:"night"`
}

// HAEntity is a Home Assistant entity with a display label (empty label:
// the entity's friendly_name).
type HAEntity struct {
	Label  string `json:"label"`
	Entity string `json:"entity"`
}

type HAConfig struct {
	URL   string `json:"url"`   // e.g. http://homeassistant.local:8123
	Token string `json:"token"` // long-lived access token, usually "$DESKUI_HA_TOKEN"
	// Buttons become toggles on the home screen mirroring these entities.
	Buttons []HAEntity `json:"buttons"`
	// Weather is a weather.* entity shown next to the clock: current
	// condition and temperature, and today's high/low from its forecast.
	Weather string `json:"weather"`
	// Sensors are shown as read-only tiles (state + unit) on the home screen.
	Sensors []HAEntity `json:"sensors"`
	// NotifyEvent is a bus event type that pops up a message on the display,
	// with event data {"title": "...", "message": "..."}. Default
	// "deskie_notify"; fire it from an automation.
	NotifyEvent string `json:"notify_event"`
}

type CalendarConfig struct {
	Feeds          []calendar.Feed `json:"feeds"`
	Days           int             `json:"days"`            // default 7
	RefreshMinutes int             `json:"refresh_minutes"` // default 10
}

// NightConfig blanks the screen to a dim clock. Night mode is on during the
// Start-End window (local "HH:MM", may wrap past midnight; both empty for
// none) and while AwayEntity reports nobody home. A tap wakes the screen for
// WakeSeconds; the Night button on the home screen enters it by hand.
type NightConfig struct {
	Start string `json:"start"`
	End   string `json:"end"`
	// AwayEntity counts as away when its state is "0" (a zone's person
	// count, e.g. zone.home), "not_home", "away" or "off". Needs Home
	// Assistant.
	AwayEntity  string `json:"away_entity"`
	WakeSeconds int    `json:"wake_seconds"` // default 60

	startMin, endMin int // parsed Start/End, minutes after midnight; -1 if unset
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, error) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("%q is not a HH:MM time", s)
	}
	return h*60 + m, nil
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{}
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields() // catch typos in hand-written config
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// Structural mistakes are fatal; missing secrets only disable the source
	// that needs them, so a fresh device still shows its UI.
	warn := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "config: "+format+" (disabled)\n", args...)
	}
	if ha := cfg.HomeAssistant; ha != nil {
		if ha.URL == "" {
			return nil, fmt.Errorf("home_assistant.url is required")
		}
		if ha.NotifyEvent == "" {
			ha.NotifyEvent = "deskie_notify"
		}
		if ha.Token, err = expandSecret(ha.Token); err != nil {
			warn("home_assistant.token: %v", err)
			cfg.HomeAssistant = nil
		}
	}
	if n := cfg.Night; n != nil {
		n.startMin, n.endMin = -1, -1
		if (n.Start == "") != (n.End == "") {
			return nil, fmt.Errorf("night: set both start and end, or neither")
		}
		if n.Start != "" {
			if n.startMin, err = parseClock(n.Start); err != nil {
				return nil, fmt.Errorf("night.start: %w", err)
			}
			if n.endMin, err = parseClock(n.End); err != nil {
				return nil, fmt.Errorf("night.end: %w", err)
			}
		}
		if n.WakeSeconds <= 0 {
			n.WakeSeconds = 60
		}
	}
	if c := cfg.Calendar; c != nil {
		var ok []calendar.Feed
		for _, f := range c.Feeds {
			if f.URL, err = expandSecret(f.URL); err != nil {
				warn("calendar feed %q: %v", f.Name, err)
				continue
			}
			ok = append(ok, f)
		}
		c.Feeds = ok
		if len(ok) == 0 {
			cfg.Calendar = nil
		}
	}
	return cfg, nil
}

// expandSecret expands $VAR references, failing on unset variables so a
// missing secret is reported at startup instead of as a confusing 401 later.
func expandSecret(s string) (string, error) {
	var missing []string
	out := os.Expand(s, func(k string) string {
		v, ok := os.LookupEnv(k)
		if !ok || v == "" {
			missing = append(missing, k)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("environment variable %s is not set", strings.Join(missing, ", "))
	}
	if out == "" {
		return "", fmt.Errorf("empty value")
	}
	return out, nil
}
