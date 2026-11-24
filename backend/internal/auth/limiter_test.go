package auth

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	l := NewLimiter()
	l.now = func() time.Time { return now }

	for range freeFailures - 1 {
		l.Fail("Anna")
		if ok, _ := l.Allow("anna"); !ok {
			t.Fatal("blocked before the last free failure")
		}
	}
	l.Fail("Anna")
	ok, wait := l.Allow("anna")
	if ok || wait != firstDelay {
		t.Fatalf("after %d failures: allowed = %v, wait = %v, want blocked for %v", freeFailures, ok, wait, firstDelay)
	}

	now = now.Add(firstDelay)
	if ok, _ := l.Allow("anna"); !ok {
		t.Fatal("still blocked after the delay")
	}
	l.Fail("anna")
	if _, wait := l.Allow("anna"); wait != 2*firstDelay {
		t.Errorf("second delay = %v, want %v", wait, 2*firstDelay)
	}

	for range 20 {
		now = now.Add(maxDelay)
		l.Fail("anna")
	}
	if _, wait := l.Allow("anna"); wait != maxDelay {
		t.Errorf("delay = %v, want the cap %v", wait, maxDelay)
	}

	l.Reset("anna")
	if ok, _ := l.Allow("anna"); !ok {
		t.Error("blocked after a reset")
	}
}

func TestLimiterForgets(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	l := NewLimiter()
	l.now = func() time.Time { return now }
	for range freeFailures + 1 {
		l.Fail("anna")
	}
	now = now.Add(forgetAfter + time.Minute)
	l.Fail("anna")
	if ok, _ := l.Allow("anna"); !ok {
		t.Error("old failures still count after an hour")
	}
}

func TestLimiterKeepsItsSize(t *testing.T) {
	l := NewLimiter()
	for i := range maxKeys + 100 {
		l.Fail(string(rune(i)) + "x")
	}
	if len(l.entries) > maxKeys {
		t.Errorf("%d keys, want at most %d", len(l.entries), maxKeys)
	}
}
