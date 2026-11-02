package main

import (
	"testing"
	"time"
)

func TestProgressLine(t *testing.T) {
	start := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	p := &progress{label: "run 1/6, webp, workers 2", total: 100, start: start}

	want := "run 1/6, webp, workers 2: 0/100 photos, 0 failed"
	if got := p.line(start.Add(5 * time.Second)); got != want {
		t.Errorf("before the first photo: got %q, want %q", got, want)
	}

	p.done.Store(25)
	p.failed.Store(1)
	want = "run 1/6, webp, workers 2: 25/100 photos, 1 failed, 2.5 photos/s, 30s left"
	if got := p.line(start.Add(10 * time.Second)); got != want {
		t.Errorf("mid-run: got %q, want %q", got, want)
	}
}
