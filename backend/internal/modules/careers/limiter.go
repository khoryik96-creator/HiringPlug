package careers

import (
	"sync"
	"time"
)

// Limits per client address: enough for a person applying to a few jobs, too
// few for a robot. Kept in memory: with several API instances each counts on
// its own, which only makes the limit looser.
const (
	burstWindow = 10 * time.Minute
	burstMax    = 5
	dayWindow   = 24 * time.Hour
	dayMax      = 20
)

type limiter struct {
	mu   sync.Mutex
	now  func() time.Time
	seen map[string][]time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, seen: map[string][]time.Time{}}
}

// Allow records an attempt from addr and says whether it is within the limits.
func (l *limiter) Allow(addr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	recent := l.seen[addr][:0]
	burst := 0
	for _, t := range l.seen[addr] {
		if now.Sub(t) < dayWindow {
			recent = append(recent, t)
			if now.Sub(t) < burstWindow {
				burst++
			}
		}
	}
	if burst >= burstMax || len(recent) >= dayMax {
		l.seen[addr] = recent
		return false
	}
	l.seen[addr] = append(recent, now)
	if len(l.seen) > 100_000 { // memory guard: forget everyone (rare; only loosens limits)
		l.seen = map[string][]time.Time{}
	}
	return true
}
