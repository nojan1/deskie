package main

import (
	"context"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"deskui/source/calendar"
	"deskui/source/hass"
	"deskui/widget"
)

// TestSnapshot renders the main screens to PNGs when DESKUI_SNAPSHOT_DIR is
// set; handy for checking layout without a device. Data is faked.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("DESKUI_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("DESKUI_SNAPSHOT_DIR not set")
	}
	cfg := &Config{
		HomeAssistant: &HAConfig{URL: "http://x", NotifyEvent: "deskie_notify"},
		Calendar:      &CalendarConfig{},
	}
	cfg.HomeAssistant.Buttons = []HAEntity{{"Lamp", "light.desk"}, {"Fan", "fan.desk"}}
	cfg.HomeAssistant.Weather = "weather.home"
	cfg.HomeAssistant.Sensors = []HAEntity{{"Indoor", "sensor.indoor"}, {"", "sensor.humidity"}, {"Washer", "sensor.washer"}}

	s := widget.NewScreen(480, 320, newTheme())
	a := newApp(context.Background(), s, cfg, func(fn func()) { fn() })
	img := image.NewRGBA(s.Bounds())
	now := time.Now()
	save := func(name string) {
		s.Tick(now)
		s.Render(img)
		f, err := os.Create(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, img)
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	at := func(day, h, m int) time.Time {
		return today.AddDate(0, 0, day).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}
	a.setHAConnected(true)
	a.setHAState(hass.State{EntityID: "light.desk", State: "on"})
	a.setHAState(hass.State{EntityID: "weather.home", State: "partlycloudy", Attributes: map[string]any{"temperature": 12.4}})
	a.setForecast(14, -3, true)
	a.setHAState(hass.State{EntityID: "sensor.indoor", State: "21.53", Attributes: map[string]any{"unit_of_measurement": "°C"}})
	a.setHAState(hass.State{EntityID: "sensor.humidity", State: "41", Attributes: map[string]any{"unit_of_measurement": "%", "friendly_name": "Humidity"}})
	a.setHAState(hass.State{EntityID: "sensor.washer", State: "running"})
	a.setEvents([]calendar.Event{
		{Title: "Birthday: Alex", Start: today, End: today.AddDate(0, 0, 1), AllDay: true},
		{Title: "Standup", Start: now.Add(20 * time.Minute), End: now.Add(35 * time.Minute)},
		{Title: "Lunch with the team", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
		{Title: "Dentist", Start: at(1, 8, 30), End: at(1, 9, 30)},
		{Title: "Standup", Start: at(1, 9, 15), End: at(1, 9, 30)},
		{Title: "Planning", Start: at(2, 13, 0), End: at(2, 14, 0)},
	})
	save("home.png")

	a.openAgenda()
	save("agenda.png")
	s.Top().Close()

	a.notify("Washing machine", "The laundry is done. Remember to hang it up before it gets wrinkly!")
	save("notify.png")
	s.Top().Close()

	a.night.Enter(now)
	save("night.png")
	a.night.wake(now)

	for _, c := range []string{"sunny", "clear-night", "partlycloudy", "cloudy", "fog", "windy", "rainy", "pouring", "snowy", "snowy-rainy", "lightning", "lightning-rainy"} {
		a.setHAState(hass.State{EntityID: "weather.home", State: c, Attributes: map[string]any{"temperature": 3.0}})
		save("weather-" + c + ".png")
	}
}
