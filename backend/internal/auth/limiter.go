package auth

import (
	"strings"
	"sync"
	"time"
)

const (
	// freeFailures is how many wrong attempts a key may make; the last one starts the delay.
	freeFailures = 5
	// firstDelay is the delay after the freeFailures-th wrong attempt; it doubles after each further one.
	firstDelay = 30 * time.Second
	maxDelay   = 15 * time.Minute
	// forgetAfter is how long after its last failure a key is forgotten.
	forgetAfter = time.Hour
	maxKeys     = 10000
	maxKeyLen   = 128
	overflowKey = "\x00overflow"
)

type attempts struct {
	failures int
	blocked  time.Time
	last     time.Time
}

// Limiter slows down repeated failed sign-ins for one key, such as a login. It is kept in
// memory, so a restart forgets it, which costs an attacker nothing worth more than the
// delay it has already imposed.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*attempts
	now     func() time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{entries: map[string]*attempts{}, now: time.Now}
}

func (l *Limiter) key(key string) string {
	key = strings.ToLower(key)
	if len(key) > maxKeyLen {
		key = key[:maxKeyLen]
	}
	if _, ok := l.entries[key]; !ok && len(l.entries) >= maxKeys {
		l.prune()
		if len(l.entries) >= maxKeys {
			return overflowKey
		}
	}
	return key
}

func (l *Limiter) prune() {
	now := l.now()
	for k, e := range l.entries {
		if now.Sub(e.last) > forgetAfter {
			delete(l.entries, k)
		}
	}
}

// Allow reports whether the key may try now, and otherwise how long it has to wait.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[l.key(key)]
	if e == nil {
		return true, 0
	}
	if wait := e.blocked.Sub(l.now()); wait > 0 {
		return false, wait
	}
	return true, 0
}

// Fail records a wrong attempt.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(key)
	e := l.entries[k]
	if e == nil {
		e = &attempts{}
		l.entries[k] = e
	}
	now := l.now()
	if now.Sub(e.last) > forgetAfter {
		e.failures = 0
	}
	e.failures++
	e.last = now
	if e.failures >= freeFailures {
		delay := firstDelay
		for i := freeFailures; i < e.failures && delay < maxDelay; i++ {
			delay *= 2
		}
		e.blocked = now.Add(min(delay, maxDelay))
	}
}

// Reset forgets a key after a correct attempt.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, l.key(key))
}
