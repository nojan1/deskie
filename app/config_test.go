package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Same shape as the JSON generated in configuration.nix.
const testConfig = `{
  "home_assistant": {
    "url": "http://homeassistant.local:8123",
    "token": "$DESKUI_TEST_TOKEN",
    "buttons": [{"label": "Lamp", "entity": "light.desk"}],
    "notify_event": "deskie_notify"
  },
  "calendar": {
    "feeds": [
      {"name": "Personal", "url": "$DESKUI_TEST_ICAL"},
      {"name": "Work", "url": "${DESKUI_TEST_MISSING}"}
    ],
    "days": 7,
    "refresh_minutes": 10
  }
}`

func writeConfig(t *testing.T, s string) string {
	p := filepath.Join(t.TempDir(), "deskui.json")
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("DESKUI_TEST_TOKEN", "tok")
	t.Setenv("DESKUI_TEST_ICAL", "https://calendar.google.com/x/basic.ics")
	cfg, err := loadConfig(writeConfig(t, testConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HomeAssistant == nil || cfg.HomeAssistant.Token != "tok" || cfg.HomeAssistant.Buttons[0].Entity != "light.desk" {
		t.Fatalf("home assistant: %+v", cfg.HomeAssistant)
	}
	// The feed with an unset variable is dropped, the other kept.
	if f := cfg.Calendar.Feeds; len(f) != 1 || f[0].URL != "https://calendar.google.com/x/basic.ics" {
		t.Fatalf("feeds: %+v", f)
	}
}

func TestLoadConfigMissingSecretsDisable(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, testConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HomeAssistant != nil || cfg.Calendar != nil {
		t.Fatalf("sources without secrets should be disabled: %+v", cfg)
	}
}

func TestLoadConfigTypo(t *testing.T) {
	if _, err := loadConfig(writeConfig(t, `{"home_asistant": {}}`)); err == nil {
		t.Fatal("unknown field should be an error")
	}
}
