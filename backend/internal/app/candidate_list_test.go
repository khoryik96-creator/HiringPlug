package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestTheListShowsStatusNoticeAndSalaryToThoseWhoMaySeeIt(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "availability": "not_looking",
		"noticePeriod": map[string]any{"value": 1, "unit": "month"}})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"},
		"expectedSalary": map[string]any{"min": "10000", "max": "12000", "currency": "MYR", "period": "month"},
	}).Status)

	find := func(token string) api.CandidateSummary {
		var page api.CandidatePage
		a.request(http.MethodGet, "/v1/candidates", token, nil).JSON(&page)
		for _, s := range page.Items {
			if s.Id == c.Id {
				return s
			}
		}
		t.Fatalf("not listed")
		return api.CandidateSummary{}
	}
	got := find(recruiter)
	assert.Equal(t, api.CandidateAvailabilityNotLooking, got.Availability.MustGet())
	assert.WithinDuration(t, time.Now(), got.AvailabilitySince.MustGet(), time.Minute, "set when the status was")
	assert.Equal(t, api.NoticePeriod{Value: 1, Unit: api.NoticePeriodUnitMonth}, got.NoticePeriod.MustGet())
	assert.Equal(t, "8500.00", got.CurrentSalary.MustGet().Amount)
	assert.Equal(t, "12000.00", got.ExpectedSalary.MustGet().Max.MustGet())
	assert.False(t, got.SalaryRestricted)

	hidden := find(sourcer)
	assert.True(t, hidden.SalaryRestricted, "a sourcer may not see salaries")
	assert.False(t, hidden.CurrentSalary.IsSpecified() && !hidden.CurrentSalary.IsNull())
	assert.Equal(t, api.NoticePeriod{Value: 1, Unit: api.NoticePeriodUnitMonth}, hidden.NoticePeriod.MustGet())
}

func TestTheStatusRemembersWhenItWasSet(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	assert.True(t, c.AvailabilitySince.IsNull(), "no status, no date")

	patch := func(body map[string]any) api.Candidate {
		var got api.Candidate
		body["version"] = c.Version
		res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&got)
		return got
	}
	c = patch(map[string]any{"availability": "looking"})
	set := c.AvailabilitySince.MustGet()
	c = patch(map[string]any{"currentTitle": "Auditor"})
	assert.True(t, set.Equal(c.AvailabilitySince.MustGet()), "another change keeps the date")
	c = patch(map[string]any{"availability": "not_looking"})
	assert.True(t, c.AvailabilitySince.MustGet().After(set) || c.AvailabilitySince.MustGet().Equal(set))
	c = patch(map[string]any{"availability": nil})
	assert.True(t, c.AvailabilitySince.IsNull(), "no status, no date")
}
