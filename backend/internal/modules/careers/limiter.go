package careers

import (
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// Limits per client address: enough for a person applying to a few jobs, too
// few for a robot.
const (
	burstWindow = 10 * time.Minute
	burstMax    = 5
	dayWindow   = 24 * time.Hour
	dayMax      = 20
)

func newLimiter(now func() time.Time) *ratelimit.Limiter {
	return ratelimit.New(now, ratelimit.Limit{Window: burstWindow, Max: burstMax}, ratelimit.Limit{Window: dayWindow, Max: dayMax})
}
