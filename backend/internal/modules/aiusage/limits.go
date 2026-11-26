package aiusage

import (
	"fmt"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Besides the month's limit, a company may cap the AI a day (all features
// together: a runaway import or loop cannot spend the month in an
// afternoon) and give a feature its own monthly limit (CV reading may not
// eat what the assistant needs). Both are in tokens. The month's limit
// still covers everything; whichever is reached first stops new AI work.

// Limits are the daily and per-feature limits as saved.
type Limits struct {
	Daily      *int64           // nil: none
	PerFeature map[string]int64 // feature -> tokens a month; a feature left out has none
	// KeepDaily, KeepPerFeature: that one was not sent, and stays as saved.
	KeepDaily, KeepPerFeature bool
}

func checkLimits(l *Limits, errs *apperr.FieldErrors) {
	if l == nil {
		return
	}
	if l.Daily != nil && (*l.Daily < 1 || *l.Daily > MaxLimitTokens) {
		errs.Add("dailyTokens", "must be between 1 and 1,000,000,000,000")
	}
	for f, n := range l.PerFeature {
		switch {
		case !IsFeature(f):
			errs.Add("featureTokens", fmt.Sprintf("%q is not a feature", f))
		case n < 1 || n > MaxLimitTokens:
			errs.Add("featureTokens."+f, "must be between 1 and 1,000,000,000,000")
		}
	}
}

// DayStart is when now's day began in the agency's time zone: the day the
// daily limit counts.
func DayStart(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// Within is what is left for a feature: the month's allowance left
// (monthLeft, after work in progress), lowered by the daily limit (today's
// use and work in progress taken off) and the feature's own monthly limit
// (its use this month and work in progress taken off). Zero or less: no new
// AI work for that feature now.
func Within(monthLeft int64, s Settings, feature string, today, featureMonth Spent, pending int64) int64 {
	left := monthLeft
	if s.Daily != nil {
		left = min(left, *s.Daily-today.Tokens()-pending)
	}
	if n, ok := s.PerFeature[feature]; ok {
		left = min(left, n-featureMonth.Tokens()-pending)
	}
	return left
}
