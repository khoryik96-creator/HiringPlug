package calendars

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalendarInputAndAllDay(t *testing.T) {
	start := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	in := Input{Title: " Review ", Start: start, End: start.Add(time.Hour), TimeZone: "Asia/Kuala_Lumpur", Attendees: []Attendee{{Email: "ONE@example.test"}, {Email: "one@example.test"}}}
	got, err := normalize(in)
	require.NoError(t, err)
	assert.Equal(t, "Review", got.Title)
	require.Len(t, got.Attendees, 1)
	in.AllDay = true
	_, err = normalize(in)
	require.Error(t, err, "UTC midnight is not midnight in Kuala Lumpur")
	in.Start = start.Add(-8 * time.Hour)
	in.End = in.Start.Add(24 * time.Hour)
	_, err = normalize(in)
	require.NoError(t, err)
	in.End = in.Start
	_, err = normalize(in)
	require.Error(t, err)
	in.End = in.Start.Add(time.Hour)
	in.TimeZone = "invalid"
	_, err = normalize(in)
	require.Error(t, err)
}
func TestCalendarSecretsCannotCrossUsersOrPurposes(t *testing.T) {
	s := &Service{config: Config{KeysSecret: []byte("test-key-for-calendar-0123456789012345")}}
	sealed, err := s.seal("t1", "u1", "tokens", tokens{AccessToken: "secret"})
	require.NoError(t, err)
	var got tokens
	require.NoError(t, s.open("t1", "u1", "tokens", sealed, &got))
	assert.Equal(t, "secret", got.AccessToken)
	for _, scope := range [][3]string{{"t2", "u1", "tokens"}, {"t1", "u2", "tokens"}, {"t1", "u1", "code"}} {
		require.Error(t, s.open(scope[0], scope[1], scope[2], sealed, &got))
	}
	state := s.signState(stateClaim{Tenant: "t1", User: "u1", Nonce: "nonce", Expires: time.Now().Add(time.Minute).Unix()})
	_, err = s.readState(state)
	require.NoError(t, err)
	_, err = s.readState(state + "x")
	require.Error(t, err)
}
func TestCalendarURLsAndStableIdentity(t *testing.T) {
	assert.Equal(t, stableTransactionID("a"), stableTransactionID("a"))
	assert.NotEqual(t, stableTransactionID("a"), stableTransactionID("b"))
	for _, raw := range []string{"javascript:alert(1)", "https://outlook.office.com.evil.test/event", "https://evil.test/", "https://user@outlook.office.com/"} {
		assert.Nil(t, safeWebURL(raw))
	}
	assert.NotNil(t, safeWebURL("https://outlook.office.com/calendar/item"))
	s := &Service{config: Config{Provider: Provider{GraphURL: "https://graph.microsoft.com/v1.0"}}}
	assert.True(t, s.validNext("https://graph.microsoft.com/v1.0/me/calendars/abc/calendarView?$skiptoken=1", "/me/calendars/abc/calendarView"))
	assert.False(t, s.validNext("https://evil.test/v1.0/me/calendars/abc/calendarView", "/me/calendars/abc/calendarView"))
}
