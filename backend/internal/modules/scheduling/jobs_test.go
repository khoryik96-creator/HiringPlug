package scheduling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestACalendarAddressThatFailsSaysSoPlainly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone.ics" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<html>Sign in</html>"))
	}))
	defer srv.Close()
	s := &Service{settings: Config{AllowPrivateNetworks: true, DefaultTimeZone: time.UTC}, now: time.Now}

	_, err := s.readCalendar(context.Background(), srv.URL+"/gone.ics", time.UTC)
	assert.EqualError(t, err, "the calendar address gave HTTP 404: check it, or make a new one")
	_, err = s.readCalendar(context.Background(), srv.URL+"/page", time.UTC)
	assert.EqualError(t, err, "the address does not give a calendar: use the private address in iCal format")
}
