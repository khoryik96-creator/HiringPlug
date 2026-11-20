package placements

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An invoice or payment marked "today" at 07:30 in Kuala Lumpur (23:30 UTC
// the day before) is dated today there, not yesterday.
func TestTodayIsTheAgencysDate(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	s := &Service{inv: Invoicing{TimeZone: kl}, now: func() time.Time { return time.Date(2026, 11, 30, 23, 30, 0, 0, time.UTC) }}
	assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), s.today(), "1 December in Kuala Lumpur: December's fees, not November's")
	d, errs, _ := Normalize(Details{StartDate: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), InvoiceStatus: Paid}, s.today())
	require.NoError(t, errs.Err())
	assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), *d.PaidOn)
}
