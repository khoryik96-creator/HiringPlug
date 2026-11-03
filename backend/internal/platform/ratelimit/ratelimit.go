// Package ratelimit counts attempts per key (a client address, an email) in
// sliding windows, to slow down robots: applications on the careers page,
// sign-in attempts, password reset requests.
//
// Counts are kept in memory: with several API instances each counts on its
// own, which only makes the limits looser.
package ratelimit

import (
	"sync"
	"time"
)

// Limit is at most Max attempts per Window.
type Limit struct {
	Window time.Duration
	Max    int
}

// Limiter checks attempts against limits.
type Limiter struct {
	mu     sync.Mutex
	now    func() time.Time
	limits []Limit
	keep   time.Duration // the longest window
	seen   map[string][]time.Time
}

// New returns a limiter applying every limit (now: time.Now, or a test clock).
func New(now func() time.Time, limits ...Limit) *Limiter {
	l := &Limiter{now: now, limits: limits, seen: map[string][]time.Time{}}
	for _, lim := range limits {
		if lim.Window > l.keep {
			l.keep = lim.Window
		}
	}
	return l
}

// Allow records an attempt for key and says whether it is within the limits
// (a refused attempt is not counted).
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	recent := l.seen[key][:0]
	for _, t := range l.seen[key] {
		if now.Sub(t) < l.keep {
			recent = append(recent, t)
		}
	}
	for _, lim := range l.limits {
		n := 0
		for _, t := range recent {
			if now.Sub(t) < lim.Window {
				n++
			}
		}
		if n >= lim.Max {
			l.seen[key] = recent
			return false
		}
	}
	l.seen[key] = append(recent, now)
	if len(l.seen) > 100_000 { // memory guard: forget everyone (rare; only loosens limits)
		l.seen = map[string][]time.Time{}
	}
	return true
}
