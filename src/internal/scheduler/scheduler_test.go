package scheduler

import (
	"testing"
	"time"
)

func TestNextRunInterval(t *testing.T) {
	from := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	next := NextRun("interval", 15, "", from)
	want := from.Add(15 * time.Minute).UnixMilli()
	if next != want {
		t.Fatalf("next = %d, want %d", next, want)
	}
	if got := NextRun("interval", 0, "", from); got != 0 {
		t.Fatalf("zero interval = %d", got)
	}
}

func TestNextRunDaily(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	from := time.Date(2026, 10, 31, 12, 0, 0, 0, location)
	next := NextRun("daily", 0, "18:30", from)
	candidate := time.Date(2026, 10, 31, 18, 30, 0, 0, location).UTC().UnixMilli()
	if next != candidate {
		t.Fatalf("next = %d, want %d", next, candidate)
	}
	next = NextRun("daily", 0, "09:00", from)
	if !time.UnixMilli(next).After(time.Date(2026, 10, 31, 12, 0, 0, 0, location)) {
		t.Fatalf("next daily must be in the future: %v", time.UnixMilli(next))
	}
	if got := NextRun("daily", 0, "99:99", from); got != 0 {
		t.Fatalf("invalid time = %d", got)
	}
	if got := NextRun("unknown", 1, "", from); got != 0 {
		t.Fatalf("unknown kind = %d", got)
	}
}

func TestParseTimeOfDay(t *testing.T) {
	hour, minute, err := ParseTimeOfDay("04:05")
	if err != nil || hour != 4 || minute != 5 {
		t.Fatalf("parse = %d:%d, %v", hour, minute, err)
	}
	invalid := []string{"", "4", "24:00", "12:60", "aa:bb", "12:34:56"}
	for _, value := range invalid {
		if _, _, err := ParseTimeOfDay(value); err == nil {
			t.Errorf("ParseTimeOfDay(%q) must fail", value)
		}
	}
}
