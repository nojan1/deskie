package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"time"

	"deskui/source/calendar"
	"deskui/source/hass"
	"deskui/widget"
)

func newTheme() *widget.Theme {
	return widget.DefaultTheme(
		mustFace(firaBold, 64),
		mustFace(firaBold, 24),
		mustFace(firaRegular, 18),
	)
}

// app owns the widgets and the data sources feeding them. Everything except
// startSources' goroutines runs on the UI goroutine; sources hand data over
// with post.
type app struct {
	s    *widget.Screen
	th   *widget.Theme
	ctx  context.Context
	post func(func())
	cfg  *Config

	ha        *hass.Client
	haDot     *widget.Box
	haButtons map[string]*widget.Button
	haStates  map[string]hass.State
	sensors   map[string][]*sensorTile

	weather     *weatherView
	weatherCond string
	hiLo        string

	night *nightMode

	events    []calendar.Event
	calLoaded bool
	next      *nextEvent
}

func newApp(ctx context.Context, s *widget.Screen, cfg *Config, post func(func())) *app {
	a := &app{
		s: s, th: s.Theme, ctx: ctx, post: post, cfg: cfg,
		haButtons: map[string]*widget.Button{},
		haStates:  map[string]hass.State{},
		sensors:   map[string][]*sensorTile{},
		night:     newNightMode(s, cfg.Night),
	}
	a.build()
	return a
}

// build assembles the home screen as the base layer.
func (a *app) build() {
	th := a.th

	title := widget.NewLabel("deskpi", th.Small, th.Text)
	title.AlignX = 0
	date := widget.NewClock("Mon 02 Jan", th.Small, th.Text)
	date.AlignX = 1

	headerRow := widget.NewHStack()
	headerRow.Padding = widget.PadXY(16, 0)
	headerRow.Spacing = 8
	if a.cfg.HomeAssistant != nil {
		// Connection indicator: dim until Home Assistant is connected.
		a.haDot = widget.NewBox(th.TextDim)
		a.haDot.Radius = 5
		dot := widget.NewVStack()
		dot.Padding = widget.Insets{Top: 14, Bottom: 14}
		dot.Add(a.haDot, widget.Flex(1))
		headerRow.Add(dot, widget.Fixed(10))
	}
	headerRow.
		Add(title, widget.Flex(1)).
		Add(date, widget.Auto)

	header := widget.NewVStack()
	header.Background = th.Header
	header.
		Add(headerRow, widget.Flex(1)).
		Add(widget.NewBox(th.Accent), widget.Fixed(2))

	a.next = &nextEvent{Label: *widget.NewLabel("", th.Small, th.TextDim), a: a}

	ha := a.cfg.HomeAssistant

	clockRow := widget.NewHStack()
	clockRow.Add(widget.NewClock("15:04:05", th.Large, th.Text), widget.Flex(1))
	if ha != nil && ha.Weather != "" {
		a.weather = newWeatherView(th)
		clockRow.Padding = widget.Insets{Left: 4, Right: 12}
		clockRow.Add(a.weather, widget.Fixed(140))
	}

	var sensors *widget.Stack
	if ha != nil && len(ha.Sensors) > 0 {
		sensors = widget.NewHStack()
		sensors.Spacing = 12
		sensors.Padding = widget.Insets{Top: 4, Left: 20, Right: 20}
		for _, e := range ha.Sensors {
			t := newSensorTile(th, e)
			a.sensors[e.Entity] = append(a.sensors[e.Entity], t)
			sensors.Add(t, widget.Flex(1))
		}
	}

	buttons := widget.NewHStack()
	buttons.Spacing = 12
	buttons.Padding = widget.Insets{Top: 10, Left: 20, Right: 20, Bottom: 20}
	if sensors != nil {
		buttons.Padding.Bottom = 14
	}
	if ha != nil && len(ha.Buttons) > 0 {
		for _, b := range ha.Buttons {
			buttons.Add(a.haToggle(b.Label, b.Entity), widget.Flex(1))
		}
	} else {
		// No Home Assistant buttons configured: local demo toggles.
		buttons.
			Add(widget.NewToggle(th, "Lamp", nil), widget.Flex(1)).
			Add(widget.NewToggle(th, "Fan", nil), widget.Flex(1))
	}
	buttons.
		Add(widget.NewButton(th, "Agenda", func(*widget.Button) { a.openAgenda() }), widget.Flex(1)).
		Add(widget.NewButton(th, "Night", func(*widget.Button) { a.night.Enter(time.Now()) }), widget.Flex(1))

	root := widget.NewVStack()
	root.Add(header, widget.Fixed(40))
	if sensors != nil {
		root.
			Add(clockRow, widget.Fixed(96)).
			Add(a.next, widget.Fixed(26)).
			Add(sensors, widget.Fixed(64))
	} else {
		root.
			Add(clockRow, widget.Fixed(110)).
			Add(a.next, widget.Fixed(30))
	}
	root.Add(buttons, widget.Flex(1))

	a.s.SetRoot(root)
}

// startSources launches the configured data sources.
func (a *app) startSources() {
	if ha := a.cfg.HomeAssistant; ha != nil {
		var entities []string
		for _, b := range ha.Buttons {
			entities = append(entities, b.Entity)
		}
		for _, s := range ha.Sensors {
			entities = append(entities, s.Entity)
		}
		if ha.Weather != "" {
			entities = append(entities, ha.Weather)
		}
		if n := a.cfg.Night; n != nil && n.AwayEntity != "" {
			entities = append(entities, n.AwayEntity)
		}
		// Each (re)connect refreshes the forecast.
		connected := make(chan struct{}, 1)
		a.ha = &hass.Client{
			URL: ha.URL, Token: ha.Token,
			Entities: entities,
			Events:   []string{ha.NotifyEvent},
			OnState:  func(s hass.State) { a.post(func() { a.setHAState(s) }) },
			OnEvent: func(e hass.Event) {
				if e.Type != ha.NotifyEvent {
					return
				}
				var d struct{ Title, Message string }
				json.Unmarshal(e.Data, &d)
				a.post(func() { a.notify(d.Title, d.Message) })
			},
			OnStatus: func(up bool, err error) {
				if err != nil {
					fmt.Fprintln(os.Stderr, "home assistant:", err)
				}
				if up {
					select {
					case connected <- struct{}{}:
					default:
					}
				}
				a.post(func() { a.setHAConnected(up) })
			},
		}
		go a.ha.Run(a.ctx)
		if ha.Weather != "" {
			go a.pollForecast(ha.Weather, connected)
		}
	}
	if c := a.cfg.Calendar; c != nil && len(c.Feeds) > 0 {
		p := &calendar.Poller{
			Feeds:    c.Feeds,
			Days:     c.Days,
			Every:    time.Duration(c.RefreshMinutes) * time.Minute,
			OnUpdate: func(evs []calendar.Event) { a.post(func() { a.setEvents(evs) }) },
			OnError: func(f calendar.Feed, err error) {
				// Don't print err as-is: it can contain the secret URL.
				fmt.Fprintf(os.Stderr, "calendar %q: %v\n", f.Name, redact(err, f.URL))
			},
		}
		go p.Run(a.ctx)
	}
}

// --- Home Assistant -------------------------------------------------------

func (a *app) haToggle(label, entity string) *widget.Button {
	b := widget.NewToggle(a.th, label, func(b *widget.Button) {
		// The toggle already flipped optimistically; HA's state_changed
		// confirms it. On failure, snap back to the last known state.
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
			defer cancel()
			if err := a.ha.CallService(ctx, "homeassistant", "toggle", entity, nil); err != nil {
				fmt.Fprintf(os.Stderr, "home assistant: toggle %s: %v\n", entity, err)
				a.post(func() { b.SetOn(a.haStates[entity].State == "on") })
			}
		}()
	})
	a.haButtons[entity] = b
	return b
}

func (a *app) setHAState(s hass.State) {
	a.haStates[s.EntityID] = s
	if b := a.haButtons[s.EntityID]; b != nil {
		b.SetOn(s.State == "on")
	}
	for _, t := range a.sensors[s.EntityID] {
		t.setState(s)
	}
	if ha := a.cfg.HomeAssistant; ha != nil && s.EntityID == ha.Weather {
		a.setWeatherState(s)
	}
	if n := a.cfg.Night; n != nil && s.EntityID == n.AwayEntity {
		a.night.setAwayState(s)
	}
}

// Tick runs every frame, before the screen's own tick.
func (a *app) Tick(now time.Time) { a.night.Tick(now) }

// Touched is called for every touch event, before the screen handles it.
func (a *app) Touched(now time.Time) { a.night.Touched(now) }

func (a *app) setHAConnected(up bool) {
	if a.haDot == nil {
		return
	}
	if up {
		a.haDot.SetColor(a.th.Accent)
	} else {
		a.haDot.SetColor(a.th.TextDim)
	}
}

// notify pops up a message (from the Home Assistant notify event).
func (a *app) notify(title, message string) {
	if title == "" {
		title = "Notification"
	}
	var layer *widget.Layer
	msg := widget.NewParagraph(message, a.th.Small, a.th.Text)
	body := widget.NewVStack()
	body.Spacing = 8
	body.Padding = widget.PadXY(6, 4)
	body.
		Add(msg, widget.Flex(1)).
		Add(widget.NewButton(a.th, "OK", func(*widget.Button) { layer.Close() }), widget.Fixed(48))
	layer = a.s.PushModal(widget.NewWindow(a.th, title, body), image.Pt(400, 230))
}

// --- Calendar -------------------------------------------------------------

func (a *app) setEvents(evs []calendar.Event) {
	a.events, a.calLoaded = evs, true
}

// nextEvent is the line under the clock: the current or next timed event.
type nextEvent struct {
	widget.Label
	a *app
}

func (n *nextEvent) Tick(now time.Time) {
	if n.a.cfg.Calendar == nil {
		return
	}
	n.SetText(describeNext(n.a.events, n.a.calLoaded, now))
}

func describeNext(evs []calendar.Event, loaded bool, now time.Time) string {
	if !loaded {
		return "Loading calendar..."
	}
	for _, e := range evs {
		if e.AllDay || !e.End.After(now) {
			continue
		}
		if !e.Start.After(now) {
			return fmt.Sprintf("Now: %s (until %s)", title(e), e.End.Format("15:04"))
		}
		when := e.Start.Format("15:04")
		if !sameDay(e.Start, now) {
			when = e.Start.Format("Mon 15:04")
		}
		return fmt.Sprintf("Next: %s %s", when, title(e))
	}
	return "Nothing scheduled"
}

func title(e calendar.Event) string {
	if e.Title == "" {
		return "(no title)"
	}
	return e.Title
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// openAgenda shows upcoming events grouped by day in a scrollable modal.
func (a *app) openAgenda() {
	th := a.th
	var layer *widget.Layer
	list := widget.NewList(th)
	now := time.Now()

	switch {
	case a.cfg.Calendar == nil:
		list.Add(widget.NewListItem(th, "No calendar configured", nil))
	case !a.calLoaded:
		list.Add(widget.NewListItem(th, "Loading...", nil))
	}
	var day time.Time
	shown := 0
	for _, e := range a.events {
		if !e.End.After(now) {
			continue
		}
		start := e.Start
		if start.Before(now) {
			start = now // ongoing multi-day events go under today
		}
		if shown == 0 || !sameDay(start, day) {
			day = start
			h := widget.NewLabel(dayHeading(start, now), th.Small, th.Accent)
			h.AlignX = 0
			row := widget.NewHStack()
			row.Padding = widget.Insets{Top: 10, Bottom: 4, Left: 16, Right: 16}
			row.Add(h, widget.Flex(1))
			list.Add(row)
		}
		when := e.Start.Format("15:04")
		if e.AllDay {
			when = "all day"
		}
		list.Add(widget.NewListItem(th, fmt.Sprintf("%-7s %s", when, title(e)), nil))
		shown++
	}
	if a.calLoaded && shown == 0 {
		list.Add(widget.NewListItem(th, "Nothing scheduled", nil))
	}

	body := widget.NewVStack()
	body.Spacing = 8
	body.
		Add(list, widget.Flex(1)).
		Add(widget.NewButton(th, "Close", func(*widget.Button) { layer.Close() }), widget.Fixed(48))
	layer = a.s.PushModal(widget.NewWindow(th, "Agenda", body), a.s.Bounds().Size().Sub(image.Pt(30, 20)))
}

func dayHeading(t, now time.Time) string {
	switch {
	case sameDay(t, now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, 1)):
		return "Tomorrow"
	default:
		return t.Format("Monday 2 Jan")
	}
}
