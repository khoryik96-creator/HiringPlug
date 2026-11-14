package jobs

import (
	"testing"
	"time"
)

// A day is the agency's day: at 7 in the morning in Kuala Lumpur it is
// already the 13th there, though still the 12th in UTC.
func TestTodayIsTheAgencysDay(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC) // 07:00 on the 13th in Kuala Lumpur
	if got := DayIn(morning, kl); !got.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("in Kuala Lumpur: %s, want the 13th", got)
	}
	if got := DayIn(morning, nil); !got.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("no zone set: %s, want UTC's day", got)
	}
	agencyZone.Store(kl)
	defer agencyZone.Store(nil)
	if today() != DayIn(time.Now(), kl) {
		t.Error("today is the agency's")
	}
}
