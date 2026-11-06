// Package calendar reads iCalendar (ICS) feeds, such as Google Calendar's
// "Secret address in iCal format", and expands them into concrete events.
//
// The secret iCal URL avoids OAuth entirely, which suits a headless device:
// Google Calendar > Settings > (calendar) > Integrate calendar > Secret
// address in iCal format. Treat the URL like a password.
package calendar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
	_ "time/tzdata" // feeds name zones (TZID); don't depend on the OS zoneinfo

	"github.com/emersion/go-ical"
)

type Feed struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Event struct {
	Title    string
	Location string
	Calendar string // Feed.Name
	Start    time.Time
	End      time.Time
	AllDay   bool // Start is local midnight; End is the following midnight
}

// Fetch downloads one feed and returns its events overlapping [from, to),
// with recurrences expanded, sorted by start time.
func Fetch(ctx context.Context, hc *http.Client, f Feed, from, to time.Time) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar %s: HTTP %s", f.Name, resp.Status)
	}
	return Parse(io.LimitReader(resp.Body, 32<<20), f.Name, from, to)
}

// Parse reads ICS data; see Fetch.
//
// Parsing is lenient: an event that can't be understood (bad dates, unknown
// time zone) is skipped rather than failing the whole feed.
func Parse(r io.Reader, name string, from, to time.Time) ([]Event, error) {
	cal, err := ical.NewDecoder(r).Decode()
	if err != nil {
		return nil, fmt.Errorf("calendar %s: %w", name, err)
	}
	// All-day (DATE) values are interpreted in the display's zone.
	loc := time.Local

	// Instances moved or edited individually carry RECURRENCE-ID: the
	// original start of the instance they replace.
	overridden := map[string]map[int64]bool{}
	for _, e := range cal.Events() {
		if p := e.Props.Get(ical.PropRecurrenceID); p != nil {
			if t, err := p.DateTime(loc); err == nil {
				uid := e.Props.Get(ical.PropUID)
				if uid == nil {
					continue
				}
				if overridden[uid.Value] == nil {
					overridden[uid.Value] = map[int64]bool{}
				}
				overridden[uid.Value][t.Unix()] = true
			}
		}
	}

	var out []Event
	add := func(ev Event) {
		if ev.Start.Before(to) && ev.End.After(from) {
			out = append(out, ev)
		}
	}
	for _, e := range cal.Events() {
		if st, _ := e.Status(); st == ical.EventCancelled {
			continue
		}
		base, err := baseEvent(e, name, loc)
		if err != nil {
			continue
		}
		set, err := e.RecurrenceSet(loc)
		if err != nil {
			continue
		}
		if set == nil || e.Props.Get(ical.PropRecurrenceID) != nil {
			add(base)
			continue
		}
		var skip map[int64]bool
		if uid := e.Props.Get(ical.PropUID); uid != nil {
			skip = overridden[uid.Value]
		}
		dur := base.End.Sub(base.Start)
		var days int // all-day spans repeat by calendar days, not hours
		if base.AllDay {
			days = int(dur.Round(24*time.Hour) / (24 * time.Hour))
		}
		// Include instances that began before from but are still running.
		for _, start := range set.Between(from.Add(-dur), to, true) {
			if skip[start.Unix()] {
				continue
			}
			ev := base
			ev.Start = start.In(time.Local)
			if base.AllDay {
				ev.End = ev.Start.AddDate(0, 0, days) // DST-safe
			} else {
				ev.End = ev.Start.Add(dur)
			}
			add(ev)
		}
	}
	Sort(out)
	return out, nil
}

func baseEvent(e ical.Event, name string, loc *time.Location) (Event, error) {
	dtstart := e.Props.Get(ical.PropDateTimeStart)
	if dtstart == nil {
		return Event{}, fmt.Errorf("event without DTSTART")
	}
	start, err := e.DateTimeStart(loc)
	if err != nil {
		return Event{}, err
	}
	end, err := e.DateTimeEnd(loc)
	if err != nil {
		return Event{}, err
	}
	allDay := dtstart.ValueType() == ical.ValueDate || len(dtstart.Value) == 8
	if !end.After(start) {
		if allDay {
			end = start.AddDate(0, 0, 1)
		} else {
			end = start
		}
	}
	title, _ := e.Props.Text(ical.PropSummary)
	where, _ := e.Props.Text(ical.PropLocation)
	return Event{
		Title:    title,
		Location: where,
		Calendar: name,
		Start:    start.In(time.Local),
		End:      end.In(time.Local),
		AllDay:   allDay,
	}, nil
}

// Sort orders by start; all-day events come first on their day.
func Sort(evs []Event) {
	slices.SortStableFunc(evs, func(a, b Event) int {
		if c := a.Start.Compare(b.Start); c != 0 {
			return c
		}
		if a.AllDay != b.AllDay {
			if a.AllDay {
				return -1
			}
			return 1
		}
		return a.End.Compare(b.End)
	})
}

// Poller fetches all feeds every Every and reports the merged result. A feed
// that fails keeps its last good events, so a network blip doesn't blank the
// display.
type Poller struct {
	Feeds []Feed
	Every time.Duration // default 10 min
	Days  int           // window length starting today; default 7
	HTTP  *http.Client  // default: 30 s timeout

	OnUpdate func([]Event)           // called from the poller's goroutine
	OnError  func(f Feed, err error) // optional
}

func (p *Poller) Run(ctx context.Context) {
	every, days, hc := p.Every, p.Days, p.HTTP
	if every <= 0 {
		every = 10 * time.Minute
	}
	if days <= 0 {
		days = 7
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	last := make([][]Event, len(p.Feeds))
	for {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		to := from.AddDate(0, 0, days)
		for i, f := range p.Feeds {
			evs, err := Fetch(ctx, hc, f, from, to)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if p.OnError != nil {
					p.OnError(f, err)
				}
				continue
			}
			last[i] = evs
		}
		if p.OnUpdate != nil {
			p.OnUpdate(Merge(last...))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// Merge combines per-feed lists into one sorted list.
func Merge(lists ...[]Event) []Event {
	var all []Event
	for _, l := range lists {
		all = append(all, l...)
	}
	Sort(all)
	return all
}
