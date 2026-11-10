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

func TestScreeningCallsAreKeptAndTheSalaryFollowsTheLatest(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, colleague := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleAdmin]
	c := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Nurul"})
	path := "/v1/candidates/" + c.Id + "/screening-calls"
	day := func(daysAgo int) string { return time.Now().UTC().AddDate(0, 0, -daysAgo).Format(time.DateOnly) }
	salary := func(version int, monthly string) map[string]any {
		return map[string]any{"expectedVersion": version, "currentSalary": map[string]any{"amount": monthly, "currency": "MYR", "period": "month"}, "expectedSalary": nil}
	}
	record := func(body map[string]any) api.ScreeningCallResult {
		t.Helper()
		res := a.request(http.MethodPost, path, recruiter.Token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var out api.ScreeningCallResult
		res.JSON(&out)
		return out
	}

	first := record(map[string]any{"version": c.Version, "notes": "RFL: wants regional scope", "calledOn": day(3), "compensation": salary(0, "9000")})
	assert.True(t, first.SalaryApplied)
	assert.Equal(t, day(3), first.Call.CalledOn.String())
	assert.Equal(t, recruiter.ID, first.Call.CalledBy, "you, when no one else is named")

	second := record(map[string]any{"version": first.Candidate.Version, "notes": "Now at RM 9,500", "calledOn": day(1), "calledBy": colleague.ID,
		"compensation": salary(1, "9500")})
	assert.True(t, second.SalaryApplied, "a later call sets the salary")
	assert.Equal(t, "9500.00", second.Candidate.Compensation.MustGet().CurrentSalary.MustGet().Amount)

	// A call from last week, logged only now: its salary does not replace the later call's.
	late := record(map[string]any{"version": second.Candidate.Version, "notes": "Old call", "calledOn": day(7), "compensation": salary(2, "8000")})
	assert.False(t, late.SalaryApplied)
	assert.Equal(t, "9500.00", late.Candidate.Compensation.MustGet().CurrentSalary.MustGet().Amount, "the salary follows the latest call")

	var calls api.ScreeningCallList
	a.request(http.MethodGet, path, recruiter.Token, nil).JSON(&calls)
	require.Len(t, calls.Items, 3)
	assert.Equal(t, []string{day(1), day(3), day(7)},
		[]string{calls.Items[0].CalledOn.String(), calls.Items[1].CalledOn.String(), calls.Items[2].CalledOn.String()}, "latest call first")
	assert.Equal(t, colleague.ID, calls.Items[0].CalledBy)
	assert.NotEmpty(t, calls.Items[0].CalledByName.MustGet())
	assert.Equal(t, []bool{true, true, false}, []bool{calls.Items[0].SalaryRecorded, calls.Items[1].SalaryRecorded, calls.Items[2].SalaryRecorded})

	var notes api.NoteList
	a.request(http.MethodGet, "/v1/notes?subjectType=candidate&subjectId="+c.Id, recruiter.Token, nil).JSON(&notes)
	require.Len(t, notes.Items, 3)
	for _, n := range notes.Items {
		if n.Id == second.NoteId.MustGet() {
			assert.Contains(t, n.Body, "Call on ", "the note says when and by whom, when that differs from who logged it")
			assert.Contains(t, n.Body, "Now at RM 9,500")
		}
	}

	t.Run("a call is by one of your users, and not in the future", func(t *testing.T) {
		v := late.Candidate.Version
		res := a.request(http.MethodPost, path, recruiter.Token, map[string]any{"version": v, "calledBy": "usr_01jbz6w0m0k8q9x2r3s4t5v6w7"})
		assert.Equal(t, []string{"calledBy"}, fieldNames(t, res.Error()))
		res = a.request(http.MethodPost, path, recruiter.Token, map[string]any{"version": v, "calledOn": time.Now().UTC().AddDate(0, 0, 5).Format(time.DateOnly)})
		assert.Equal(t, []string{"calledOn"}, fieldNames(t, res.Error()))
	})
}
