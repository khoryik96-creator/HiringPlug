package calendars

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
)

func TestOutlookPagingNeverForwardsCredentialsToAnotherHost(t *testing.T) {
	for _, kind := range []string{"foreign", "repeat", "duplicate", "missing"} {
		t.Run(kind, func(t *testing.T) {
			count := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				assert.Equal(t, "Bearer private-access", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				event := `{"id":"one","start":{"dateTime":"2026-11-18T10:00:00.0000000","timeZone":"UTC"},"end":{"dateTime":"2026-11-18T11:00:00.0000000","timeZone":"UTC"}}`
				switch kind {
				case "missing":
					_, _ = w.Write([]byte(`{"unexpected":[]}`))
				case "foreign":
					_, _ = fmt.Fprintf(w, `{"value":[%s],"@odata.nextLink":"https://untrusted.test/steal"}`, event)
				case "repeat":
					_, _ = fmt.Fprintf(w, `{"value":[%s],"@odata.nextLink":%q}`, event, server.URL+r.URL.RequestURI())
				case "duplicate":
					_, _ = fmt.Fprintf(w, `{"value":[%s,%s]}`, event, event)
				}
			}))
			defer server.Close()
			m := New(nil)
			m.ConnectOutlookWith(Deps{}, Config{HTTP: server.Client(), Provider: Provider{AuthorizeURL: server.URL, TokenURL: server.URL, GraphURL: server.URL}})
			_, err := m.service.readEvents(context.Background(), store.CalendarConnection{CalendarID: "default"}, "private-access", time.Now(), time.Now().Add(24*time.Hour))
			require.Error(t, err)
			assert.Equal(t, 1, count, "bad continuation must not make another provider request")
		})
	}
}
func TestOutlookIntervalsAndLocalAllDayPayload(t *testing.T) {
	instant, err := (graphTime{DateTime: "2026-11-18T09:00:00.0000000", TimeZone: "UTC"}).instant()
	require.NoError(t, err)
	assert.Equal(t, 9, instant.Hour())
	_, err = (graphTime{DateTime: "2026-11-18T09:00:00", TimeZone: "unknown"}).instant()
	require.Error(t, err)
	// The DST boundary is accepted through explicit offsets, not guessed from a floating time.
	a, err := (graphTime{DateTime: "2026-11-01T01:30:00-04:00"}).instant()
	require.NoError(t, err)
	b, err := (graphTime{DateTime: "2026-11-01T01:30:00-05:00"}).instant()
	require.NoError(t, err)
	assert.Equal(t, time.Hour, b.Sub(a))
	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	start := time.Date(2026, 11, 18, 0, 0, 0, 0, loc)
	raw, err := eventBody(store.CalendarEvent{ID: "event", Title: "Day off", StartsAt: start, EndsAt: start.Add(24 * time.Hour), TimeZone: loc.String(), AllDay: true, Attendees: []byte(`[]`)})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"dateTime":"2026-11-18T00:00:00"`)
	assert.Contains(t, string(raw), `"timeZone":"Asia/Kuala_Lumpur"`)
	assert.NotContains(t, strings.ToLower(string(raw)), "candidate")
}

func TestOutlookNetworkErrorsAreRetryableAndContainNoRequestURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	s := &Service{config: Config{HTTP: &http.Client{Timeout: time.Second}}}
	_, err := s.request(context.Background(), "GET", server.URL+"/private-calendar?secret=value", "private-access", nil, "")
	var provider *providerError
	require.ErrorAs(t, err, &provider)
	assert.Equal(t, 0, provider.Status)
	assert.NotContains(t, err.Error(), server.URL)
	assert.NotContains(t, err.Error(), "private-access")
}
