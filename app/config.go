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
}

type HAConfig struct {
	URL   string `json:"url"`   // e.g. http://homeassistant.local:8123
	Token string `json:"token"` // long-lived access token, usually "$DESKUI_HA_TOKEN"
	// Buttons become toggles on the home screen mirroring these entities.
	Buttons []struct {
		Label  string `json:"label"`
		Entity string `json:"entity"`
	} `json:"buttons"`
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
