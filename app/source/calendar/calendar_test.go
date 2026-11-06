package calendar

import (
	"strings"
	"testing"
	"time"
)

const sample = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Google Inc//Google Calendar 70.9054//EN
BEGIN:VEVENT
UID:standup
DTSTART;TZID=Europe/Stockholm:20261102T091500
DTEND;TZID=Europe/Stockholm:20261102T093000
RRULE:FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Standup
END:VEVENT
BEGIN:VEVENT
UID:standup
RECURRENCE-ID;TZID=Europe/Stockholm:20261109T091500
DTSTART;TZID=Europe/Stockholm:20261109T100000
DTEND;TZID=Europe/Stockholm:20261109T101500
SUMMARY:Standup (moved)
END:VEVENT
BEGIN:VEVENT
UID:sync
DTSTART;TZID=Europe/Stockholm:20260908T140000
DTEND;TZID=Europe/Stockholm:20260908T150000
RRULE:FREQ=MONTHLY;BYDAY=2TU
EXDATE;TZID=Europe/Stockholm:20261013T140000
SUMMARY:Monthly sync
END:VEVENT
BEGIN:VEVENT
UID:bday
DTSTART;VALUE=DATE:20261106
DTEND;VALUE=DATE:20261107
SUMMARY:Birthday
END:VEVENT
BEGIN:VEVENT
UID:lunch
DTSTART:20261106T110000Z
DTEND:20261106T120000Z
SUMMARY:Lunch
LOCATION:Cafe
END:VEVENT
BEGIN:VEVENT
UID:gone
DTSTART:20261106T150000Z
DTEND:20261106T160000Z
STATUS:CANCELLED
SUMMARY:Cancelled thing
END:VEVENT
END:VCALENDAR
`

func TestParse(t *testing.T) {
	sthlm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = sthlm
	defer func() { time.Local = old }()

	from := time.Date(2026, 11, 6, 0, 0, 0, 0, sthlm) // a Friday
	to := from.AddDate(0, 0, 5)
	evs, err := Parse(strings.NewReader(strings.ReplaceAll(sample, "\n", "\r\n")), "work", from, to)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, e := range evs {
		got = append(got, e.Start.Format("Mon 15:04")+" "+e.Title)
	}
	want := []string{
		"Fri 00:00 Birthday",
		"Fri 09:15 Standup",
		"Fri 12:00 Lunch",           // 11:00Z in CET
		"Mon 10:00 Standup (moved)", // override replaces the 09:15 instance
		"Tue 09:15 Standup",
		"Tue 14:00 Monthly sync", // 2nd Tuesday of November, after DST ended
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	bd := evs[0]
	if !bd.AllDay || !bd.End.Equal(time.Date(2026, 11, 7, 0, 0, 0, 0, sthlm)) {
		t.Errorf("all-day event: allDay=%v end=%v", bd.AllDay, bd.End)
	}
	if evs[2].Location != "Cafe" || evs[2].Calendar != "work" {
		t.Errorf("lunch fields: %+v", evs[2])
	}
}
