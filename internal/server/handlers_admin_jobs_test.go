package server

import (
	"testing"
	"time"
)

func TestEstimateAnimeCoverETA(t *testing.T) {
	pace := 650 * time.Millisecond
	if got := estimateAnimeCoverETA(0, 0, time.Time{}, pace); got != 0 {
		t.Fatalf("empty pending: %d", got)
	}
	got := estimateAnimeCoverETA(10, 0, time.Time{}, pace)
	want := int(10 * pace.Seconds())
	if got != want {
		t.Fatalf("pace-based ETA: got %d want %d", got, want)
	}
	started := time.Now().Add(-10 * time.Second)
	got = estimateAnimeCoverETA(5, 5, started, pace)
	if got <= 0 {
		t.Fatalf("observed ETA should be positive, got %d", got)
	}
}

func TestFormatDurationSeconds(t *testing.T) {
	if got := formatDurationSeconds(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	sec := 30
	if got := formatDurationSeconds(&sec); got != "< 1 min" {
		t.Fatalf("30s: %q", got)
	}
	sec = 125
	if got := formatDurationSeconds(&sec); got != "2 min" {
		t.Fatalf("125s: %q", got)
	}
	sec = 3700
	if got := formatDurationSeconds(&sec); got != "1 h 1 min" {
		t.Fatalf("3700s: %q", got)
	}
}
