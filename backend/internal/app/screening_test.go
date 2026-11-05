package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestAScreeningCallUpdatesTheCandidate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "nationality": "MY"})
	path := "/v1/candidates/" + c.Id + "/screening-calls"
	salary := map[string]any{
		"expectedVersion": 0,
		"currentSalary":   map[string]any{"amount": "8500", "currency": "MYR", "period": "month"},
		"expectedSalary":  map[string]any{"min": "9500", "max": nil, "currency": "SGD", "period": "month"},
	}

	call := map[string]any{
		"version":           c.Version,
		"notes":             "  Open to Singapore; wants hybrid.  ",
		"nationality":       "SG",
		"noticePeriod":      map[string]any{"value": 2, "unit": "week"},
		"workAuthorization": map[string]any{"status": "requires_visa"},
		"source":            "referral",
		"compensation":      salary,
	}
	res := a.request(http.MethodPost, path, token, call)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var got api.ScreeningCallResult
	res.JSON(&got)
	after := got.Candidate
	assert.Equal(t, "SG", after.Nationality.MustGet())
	assert.Equal(t, api.NoticePeriod{Value: 2, Unit: "week"}, after.NoticePeriod.MustGet())
	assert.Equal(t, api.WorkAuthorizationStatus("requires_visa"), after.WorkAuthorization.MustGet().Status)
	assert.Equal(t, api.CandidateSource("referral"), after.Source.MustGet(), "set: the candidate had no source")
	assert.False(t, got.SourceKept)
	comp := after.Compensation.MustGet()
	assert.Equal(t, "SGD", comp.ExpectedSalary.MustGet().Currency, "salary and its currency saved")
	assert.Equal(t, "8500.00", comp.CurrentSalary.MustGet().Amount)

	var notes api.NoteList
	a.request(http.MethodGet, "/v1/notes?subjectType=candidate&subjectId="+c.Id, token, nil).JSON(&notes)
	require.Len(t, notes.Items, 1)
	assert.Equal(t, got.NoteId.MustGet(), notes.Items[0].Id)
	assert.Equal(t, api.NoteKind("screening"), notes.Items[0].Kind)
	assert.Equal(t, "Open to Singapore; wants hybrid.", notes.Items[0].Body)

	t.Run("an existing source is kept, and fields left out are not changed", func(t *testing.T) {
		res := a.request(http.MethodPost, path, token, map[string]any{"version": after.Version, "source": "jobstreet"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var again api.ScreeningCallResult
		res.JSON(&again)
		assert.True(t, again.SourceKept)
		assert.Equal(t, api.CandidateSource("referral"), again.Candidate.Source.MustGet())
		assert.Equal(t, "SG", again.Candidate.Nationality.MustGet())
		assert.True(t, again.NoteId.IsNull(), "no notes, no note")
		after = again.Candidate
	})

	t.Run("an out-of-date version is a conflict", func(t *testing.T) {
		res := a.request(http.MethodPost, path, token, map[string]any{"version": c.Version, "nationality": "ID"})
		assert.Equal(t, http.StatusConflict, res.Status)
	})

	t.Run("nothing is saved when a part is refused", func(t *testing.T) {
		bad := map[string]any{"expectedVersion": comp.Version, "currentSalary": nil,
			"expectedSalary": map[string]any{"min": "9000", "max": "8000", "currency": "MYR", "period": "month"}}
		res := a.request(http.MethodPost, path, token, map[string]any{"version": after.Version, "nationality": "ID", "notes": "x", "compensation": bad})
		require.Equal(t, http.StatusBadRequest, res.Status)
		stale := map[string]any{"expectedVersion": comp.Version + 5, "currentSalary": nil, "expectedSalary": nil}
		res = a.request(http.MethodPost, path, token, map[string]any{"version": after.Version, "nationality": "ID", "notes": "y", "compensation": stale})
		require.Equal(t, http.StatusConflict, res.Status, "the salary was changed meanwhile")

		var now api.Candidate
		a.request(http.MethodGet, "/v1/candidates/"+c.Id, token, nil).JSON(&now)
		assert.Equal(t, "SG", now.Nationality.MustGet(), "the profile change was rolled back with the salary")
		var notes api.NoteList
		a.request(http.MethodGet, "/v1/notes?subjectType=candidate&subjectId="+c.Id, token, nil).JSON(&notes)
		assert.Len(t, notes.Items, 1, "and so was the note")
	})

	t.Run("salary needs its own permission, and viewers cannot record calls", func(t *testing.T) {
		sourcer := tenant.Users[api.RoleSourcer].Token
		res := a.request(http.MethodPost, path, sourcer, map[string]any{"version": after.Version, "nationality": "ID", "compensation": salary})
		assert.Equal(t, http.StatusForbidden, res.Status)
		res = a.request(http.MethodPost, path, sourcer, map[string]any{"version": after.Version, "notes": "Call back Friday", "sensitive": true})
		assert.Equal(t, []string{"sensitive"}, fieldNames(t, res.Error()), "sourcers may not write sensitive notes")
		res = a.request(http.MethodPost, path, tenant.Users[api.RoleViewer].Token, map[string]any{"version": after.Version})
		assert.Equal(t, http.StatusForbidden, res.Status)
	})

	t.Run("the audit log says a call was recorded, never the salary", func(t *testing.T) {
		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT string_agg(changes::text, ' ') FROM audit_log WHERE entity_id = $1 AND action = 'candidate.screened'`, c.Id).Scan(&changes))
		assert.Contains(t, changes, `"salary": true`)
		assert.NotContains(t, changes, "8500")
		assert.NotContains(t, changes, "9500")
	})
}
