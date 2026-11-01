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

func TestCreateCandidateNormalizesInput(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	c := a.createCandidate(token, map[string]any{
		"firstName":   "  Siti   Nur ",
		"lastName":    "Aisyah",
		"email":       "Siti.Aisyah@Example.COM",
		"phone":       "012-345 6789",
		"nationality": "MY",
		"workAuthorization": map[string]any{
			"status": "local",
		},
		"noticePeriod":   map[string]any{"value": 1, "unit": "month"},
		"industry":       "Banking",
		"specialization": "Audit & Taxation",
		"source":         "jobstreet",
	})

	assert.Equal(t, "Siti Nur", c.FirstName)
	assert.Equal(t, "Siti Nur Aisyah", c.FullName)
	assert.Equal(t, "siti.aisyah@example.com", string(c.Email.MustGet()))
	assert.Equal(t, "+60123456789", c.Phone.MustGet())
	assert.Equal(t, api.WorkAuthorizationStatusLocal, c.WorkAuthorization.MustGet().Status)
	assert.Equal(t, 1, c.Version)
	assert.True(t, c.CurrentTitle.IsNull(), "unset optional fields are null, not empty strings")
}

func TestCandidateValidationReportsEveryField(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"missing first name", map[string]any{"lastName": "Tan"}, []string{"firstName"}},
		{"blank first name", map[string]any{"firstName": "   "}, []string{"firstName"}},
		{"unknown field (typo)", map[string]any{"firstName": "Wei", "fristName": "Wei"}, []string{"fristName"}},
		{"lower-case country", map[string]any{"firstName": "Wei", "nationality": "my"}, []string{"nationality"}},
		{"bad phone", map[string]any{"firstName": "Wei", "phone": "12"}, []string{"phone"}},
		{
			"expat without basis",
			map[string]any{"firstName": "Wei", "workAuthorization": map[string]any{"status": "expat_eligible"}},
			[]string{"workAuthorization.basis"},
		},
		{
			"local with basis",
			map[string]any{"firstName": "Wei", "workAuthorization": map[string]any{"status": "local", "basis": "rp_t"}},
			[]string{"workAuthorization.basis"},
		},
		{"notice period too long", map[string]any{"firstName": "Wei", "noticePeriod": map[string]any{"value": 400, "unit": "day"}}, []string{"noticePeriod.value"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := a.request(http.MethodPost, "/v1/candidates", token, tc.body)
			require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
			assert.ElementsMatch(t, tc.fields, fieldNames(t, res.Error()))
		})
	}
}

func TestExpatWithRightToWork(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	c := a.createCandidate(token, map[string]any{
		"firstName":   "Arjun",
		"nationality": "IN",
		"workAuthorization": map[string]any{
			"status": "expat_eligible", "basis": "rp_t", "note": "RP-T valid until 2031",
		},
	})
	wa := c.WorkAuthorization.MustGet()
	assert.Equal(t, api.WorkAuthorizationBasisRpT, wa.Basis.MustGet())
	assert.Equal(t, "RP-T valid until 2031", wa.Note.MustGet())
}

func TestDuplicateEmailIsRejectedWithTheExistingCandidate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	first := a.createCandidate(token, map[string]any{"firstName": "Mei", "email": "mei@example.com"})
	res := a.request(http.MethodPost, "/v1/candidates", token, map[string]any{"firstName": "Mei Ling", "email": "MEI@example.com"})

	require.Equal(t, http.StatusConflict, res.Status)
	e := res.Error()
	assert.Equal(t, api.ErrorCodeCANDIDATEDUPLICATEEMAIL, e.Error.Code)
	assert.Equal(t, first.Id, (*e.Error.Details)["existingCandidateId"])
}

func TestUpdateCandidate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Hafiz", "currentTitle": "Accountant", "location": "Kuala Lumpur"})
	path := "/v1/candidates/" + c.Id

	t.Run("changes only the fields sent; null clears a field", func(t *testing.T) {
		res := a.request(http.MethodPatch, path, token, map[string]any{
			"version": 1, "currentTitle": "Senior Accountant", "location": nil,
		})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var updated api.Candidate
		res.JSON(&updated)
		assert.Equal(t, 2, updated.Version)
		assert.Equal(t, "Senior Accountant", updated.CurrentTitle.MustGet())
		assert.True(t, updated.Location.IsNull())
		assert.Equal(t, "Hafiz", updated.FirstName)
	})

	t.Run("a stale version is refused instead of overwriting", func(t *testing.T) {
		res := a.request(http.MethodPatch, path, token, map[string]any{"version": 1, "currentTitle": "Finance Manager"})
		require.Equal(t, http.StatusConflict, res.Status)
		e := res.Error()
		assert.Equal(t, api.ErrorCodeCONFLICTSTALEVERSION, e.Error.Code)
		assert.EqualValues(t, 2, (*e.Error.Details)["currentVersion"])
	})

	t.Run("every change is in the audit log with its request ID", func(t *testing.T) {
		var action, requestID string
		var changes map[string]any
		err := testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT action, request_id, changes FROM audit_log WHERE entity_id = $1 AND action = 'candidate.updated'`, c.Id,
		).Scan(&action, &requestID, &changes)
		require.NoError(t, err)
		assert.NotEmpty(t, requestID)
		assert.Contains(t, changes, "currentTitle")
		assert.Contains(t, changes, "location")
		assert.NotContains(t, changes, "firstName", "unchanged fields are not logged")
	})
}

func TestListAndSearchCandidates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleViewer, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	for _, name := range []string{"José", "Aminah", "Kumar"} {
		a.createCandidate(token, map[string]any{"firstName": name, "lastName": "Test"})
	}

	t.Run("search ignores case and accents", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/candidates?q=JOSE", token, nil)
		require.Equal(t, http.StatusOK, res.Status)
		var page api.CandidatePage
		res.JSON(&page)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "José Test", page.Items[0].FullName)
	})

	t.Run("pages are newest first and link to the next page", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/candidates?limit=2", token, nil)
		var first api.CandidatePage
		res.JSON(&first)
		require.Len(t, first.Items, 2)
		assert.Equal(t, "Kumar Test", first.Items[0].FullName)
		next := first.NextCursor.MustGet()

		res = a.request(http.MethodGet, "/v1/candidates?limit=2&cursor="+next, token, nil)
		var second api.CandidatePage
		res.JSON(&second)
		require.Len(t, second.Items, 1)
		assert.Equal(t, "José Test", second.Items[0].FullName)
		assert.True(t, second.NextCursor.IsNull())
	})
}

func TestSalaryIsOnlyVisibleWithPermission(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Daniel"})
	compPath := "/v1/candidates/" + c.Id + "/compensation"

	res := a.request(http.MethodPut, compPath, recruiter, map[string]any{
		"expectedVersion": 0,
		"currentSalary":   map[string]any{"amount": "8500", "currency": "MYR", "period": "month"},
		"expectedSalary":  map[string]any{"min": "10000", "max": "11000.50", "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var comp api.Compensation
	res.JSON(&comp)
	assert.Equal(t, 1, comp.Version)
	assert.Equal(t, "8500.00", comp.CurrentSalary.MustGet().Amount)
	assert.Equal(t, "11000.50", comp.ExpectedSalary.MustGet().Max.MustGet())

	t.Run("recruiters see it", func(t *testing.T) {
		body := a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter, nil).Map()
		require.Contains(t, body, "compensation")
		assert.Empty(t, body["restrictedFields"])
	})

	for _, role := range []api.Role{api.RoleSourcer, api.RoleViewer} {
		t.Run(string(role)+"s do not, and are told it is restricted", func(t *testing.T) {
			body := a.request(http.MethodGet, "/v1/candidates/"+c.Id, tenant.Users[role].Token, nil).Map()
			assert.NotContains(t, body, "compensation")
			assert.Equal(t, []any{"compensation"}, body["restrictedFields"])
		})
	}

	t.Run("sourcers cannot change it", func(t *testing.T) {
		res := a.request(http.MethodPut, compPath, tenant.Users[api.RoleSourcer].Token, map[string]any{
			"expectedVersion": 1, "currentSalary": nil, "expectedSalary": nil,
		})
		require.Equal(t, http.StatusForbidden, res.Status)
		assert.Equal(t, api.ErrorCodeFORBIDDEN, res.Error().Error.Code)
	})

	t.Run("a stale version is refused", func(t *testing.T) {
		res := a.request(http.MethodPut, compPath, recruiter, map[string]any{
			"expectedVersion": 0, "currentSalary": nil, "expectedSalary": nil,
		})
		require.Equal(t, http.StatusConflict, res.Status)
	})

	t.Run("a backwards range is refused", func(t *testing.T) {
		res := a.request(http.MethodPut, compPath, recruiter, map[string]any{
			"expectedVersion": 1, "currentSalary": nil,
			"expectedSalary": map[string]any{"min": "9000", "max": "8000", "currency": "MYR", "period": "month"},
		})
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.Equal(t, []string{"expectedSalary.max"}, fieldNames(t, res.Error()))
	})

	t.Run("the audit log records the change but never the amounts", func(t *testing.T) {
		var changes string
		err := testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT changes::text FROM audit_log WHERE entity_id = $1 AND action = 'candidate.compensation_updated'`, c.Id,
		).Scan(&changes)
		require.NoError(t, err)
		assert.NotContains(t, changes, "8500")
		assert.Contains(t, changes, "redacted")
	})
}

func TestTenantsCannotSeeEachOther(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tokenA := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	tokenB := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	c := a.createCandidate(tokenA, map[string]any{"firstName": "Private", "email": "private@example.com"})

	res := a.request(http.MethodGet, "/v1/candidates/"+c.Id, tokenB, nil)
	assert.Equal(t, http.StatusNotFound, res.Status)

	var page api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates", tokenB, nil).JSON(&page)
	assert.Empty(t, page.Items)

	res = a.request(http.MethodPatch, "/v1/candidates/"+c.Id, tokenB, map[string]any{"version": 1, "firstName": "Hacked"})
	assert.Equal(t, http.StatusNotFound, res.Status)

	// The same email is free in another tenant.
	a.createCandidate(tokenB, map[string]any{"firstName": "Other", "email": "private@example.com"})
}
