package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestABookingLinkAsksForTheirPhoneAndEmail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	var hours []map[string]any
	for d := 1; d <= 7; d++ {
		hours = append(hours, map[string]any{"day": d, "start": "08:00", "end": "18:00"})
	}
	res := a.request(http.MethodPut, "/v1/scheduling/me", recruiter, map[string]any{
		"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 3,
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	link := a.bookingLink(recruiter, map[string]any{
		"candidateId": cand.Id, "title": "Phone call", "durationMinutes": 15, "location": "Phone call: we call you", "askFor": []string{"phone", "email"},
	})
	assert.Equal(t, []api.BookingAsk{api.BookingAskPhone, api.BookingAskEmail}, link.AskFor)
	token := tokenInURL.FindStringSubmatch(link.Url.MustGet())[1]
	page := a.publicBooking(token)
	assert.Equal(t, []api.BookingAsk{api.BookingAskPhone, api.BookingAskEmail}, page.AskFor, "the page asks for them")
	require.NotEmpty(t, page.Slots)

	res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": page.Slots[0]})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Contains(t, string(res.Body), "is needed to call you")
	res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": page.Slots[0], "phone": "call me", "email": "siti@"})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{
		"startsAt": page.Slots[0], "phone": " +60 12-345  6789 ", "email": "Siti@Example.com",
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&page)
	assert.Empty(t, page.AskFor, "given: not asked again when moving the time")

	var list struct{ Items []api.BookingLink }
	a.request(http.MethodGet, "/v1/booking-links?candidateId="+cand.Id, recruiter, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "+60 12-345 6789", list.Items[0].GivenPhone.MustGet(), "the recruiter sees what they gave")
	assert.Equal(t, "siti@example.com", list.Items[0].GivenEmail.MustGet())
}
