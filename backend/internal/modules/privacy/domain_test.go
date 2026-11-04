package privacy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(s string) *string { return &s }

func TestConsentAndRequestsAreTidiedAndNotInTheFuture(t *testing.T) {
	today := day("2026-11-04")
	in, err := CheckConsent(ConsentInput{Status: Given, Method: "phone", Day: today, Note: ptr("  agreed on a call  ")}, today)
	require.NoError(t, err)
	assert.Equal(t, "agreed on a call", *in.Note)
	in, err = CheckConsent(ConsentInput{Status: Given, Method: "phone", Day: today, Note: ptr("   ")}, today)
	require.NoError(t, err)
	assert.Nil(t, in.Note, "a blank note is no note")
	_, err = CheckConsent(ConsentInput{Status: Given, Method: "phone", Day: today.AddDate(0, 0, 1)}, today)
	require.Error(t, err)

	r, err := CheckRequest(RequestInput{Kind: KindAccess, ReceivedOn: today, Details: ptr(" by email ")}, today)
	require.NoError(t, err)
	assert.Equal(t, "by email", *r.Details)
	_, err = CheckRequest(RequestInput{Kind: KindAccess, ReceivedOn: today.AddDate(0, 0, 2)}, today)
	require.Error(t, err)
}

func TestClosingARequestNeedsItsOutcome(t *testing.T) {
	_, err := CheckDecision(Decision{Status: StatusDone, Outcome: ptr("  ")})
	require.Error(t, err, "say what was done")
	_, err = CheckDecision(Decision{Status: StatusDeclined})
	require.Error(t, err)
	d, err := CheckDecision(Decision{Status: StatusDone, Outcome: ptr(" Sent a copy by email. ")})
	require.NoError(t, err)
	assert.Equal(t, "Sent a copy by email.", *d.Outcome)
	_, err = CheckDecision(Decision{Status: StatusOpen})
	require.NoError(t, err, "reopening needs no outcome")
}

func TestAccessAndCorrectionRequestsAreDueIn21Days(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	r := Request{Kind: KindAccess, Status: StatusOpen, ReceivedOn: day("2026-10-01")}
	assert.Equal(t, day("2026-10-22"), *DueOn(r))
	assert.False(t, Overdue(r, day("2026-10-22")), "due today is not late")
	assert.True(t, Overdue(r, day("2026-10-23")))
	assert.Equal(t, 22, DaysOpen(r, day("2026-10-23"), kl))

	r.Kind = KindCorrection
	assert.NotNil(t, DueOn(r))
	r.Kind = "removal"
	assert.Nil(t, DueOn(r), "no fixed time for other requests")
	assert.False(t, Overdue(r, day("2027-01-01")))

	closed := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC) // 6 Oct in Kuala Lumpur
	r = Request{Kind: KindAccess, Status: StatusDone, ReceivedOn: day("2026-10-01"), ClosedAt: &closed}
	assert.False(t, Overdue(r, day("2026-12-01")), "closed requests are never late")
	assert.Equal(t, 5, DaysOpen(r, day("2026-12-01"), kl), "counted to the day it was closed, in the company's time zone")
	assert.Zero(t, DaysOpen(Request{ReceivedOn: day("2026-10-02")}, day("2026-10-01"), kl))
}

func TestAMergedCandidateKeepsWithdrawnConsent(t *testing.T) {
	given := Consent{Status: Given, Method: MethodCareersForm, Day: day("2026-01-01")}
	withdrawn := Consent{Status: Withdrawn, Method: "email", Day: day("2026-02-01")}

	out, changed := Merged(given, nil)
	assert.True(t, changed)
	assert.Equal(t, given, out, "the only consent moves over")
	out, changed = Merged(withdrawn, &given)
	assert.True(t, changed)
	assert.Equal(t, Withdrawn, out.Status, "if either withdrew, do not contact")
	out, changed = Merged(given, &withdrawn)
	assert.False(t, changed)
	assert.Equal(t, Withdrawn, out.Status)
	_, changed = Merged(given, &given)
	assert.False(t, changed)
}

func TestDayIsTheCalendarDayInTheCompanysTimeZone(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	assert.Equal(t, day("2026-11-05"), Day(time.Date(2026, 11, 4, 17, 30, 0, 0, time.UTC), kl))
}
