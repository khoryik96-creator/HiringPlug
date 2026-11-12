package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestScreeningKeepsSalaryNotesAndEarlierCallFigures(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleViewer)
	owner := tenant.Users[api.RoleOwner].Token
	c := a.createCandidate(owner, map[string]any{"firstName": "Synthetic Salary History"})
	call := func(amount, notes, date string, version int) api.ScreeningCallResult {
		r := a.request("POST", "/v1/candidates/"+c.Id+"/screening-calls", owner, map[string]any{"version": c.Version, "calledOn": date, "compensation": map[string]any{"expectedVersion": version, "currentSalary": map[string]any{"amount": amount, "currency": "MYR", "period": "month", "description": "Recorded package " + notes}, "expectedSalary": nil, "salaryNotes": notes}})
		require.Equal(t, 200, r.Status, "%s", r.Body)
		var result api.ScreeningCallResult
		r.JSON(&result)
		return result
	}
	old := call("10000", "Original salary note", "2026-10-01", 0)
	latest := call("12000", "New salary note", "2026-10-02", old.Candidate.Compensation.MustGet().Version)
	earlier := call("9000", "Backdated salary note", "2026-09-01", 0)
	assert.False(t, earlier.SalaryApplied)
	require.NotNil(t, earlier.Call.SalarySnapshot)
	assert.Equal(t, "9000", earlier.Call.SalarySnapshot.CurrentSalary.MustGet().Amount)
	// A call where salary was not discussed shows no salary as its own.
	r := a.request("POST", "/v1/candidates/"+c.Id+"/screening-calls", owner, map[string]any{"version": c.Version, "calledOn": "2026-10-03", "notes": "Discussed notice period only."})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var noSalary api.ScreeningCallResult
	r.JSON(&noSalary)
	assert.Nil(t, noSalary.Call.SalarySnapshot, "no salary was recorded on this call")
	var history api.ScreeningCallList
	a.request("GET", "/v1/candidates/"+c.Id+"/screening-calls", owner, nil).JSON(&history)
	require.Len(t, history.Items, 4)
	byID := map[string]api.ScreeningCallSummary{}
	for _, h := range history.Items {
		byID[h.Id] = h
	}
	require.NotNil(t, byID[old.Call.Id].SalarySnapshot)
	assert.Equal(t, "Original salary note", byID[old.Call.Id].SalarySnapshot.SalaryNotes.MustGet())
	assert.Equal(t, "10000.00", byID[old.Call.Id].SalarySnapshot.CurrentSalary.MustGet().Amount)
	assert.Equal(t, "New salary note", byID[latest.Call.Id].SalarySnapshot.SalaryNotes.MustGet())
	// Even a salary-authorized database session cannot rewrite an earlier call.
	ctx := context.Background()
	require.NoError(t, database.InTx(ctx, testutil.Current.App, authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleOwner), func(tx pgx.Tx) error {
		changed, err := tx.Exec(ctx, `UPDATE screening_call_salary SET snapshot='{}'::jsonb WHERE call_id=$1`, old.Call.Id)
		if err == nil {
			assert.Zero(t, changed.RowsAffected())
		}
		return err
	}))
	// Older clients changing a figure do not erase the free salary notes.
	r = a.request("PUT", "/v1/candidates/"+c.Id+"/compensation", owner, map[string]any{"expectedVersion": latest.Candidate.Compensation.MustGet().Version, "currentSalary": map[string]any{"amount": "13000", "currency": "MYR", "period": "month"}, "expectedSalary": nil})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var comp api.Compensation
	r.JSON(&comp)
	assert.Equal(t, "New salary note", comp.SalaryNotes.MustGet())
	r = a.request("GET", "/v1/candidates/"+c.Id+"/screening-calls", tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, 200, r.Status)
	assert.NotContains(t, string(r.Body), "salarySnapshot")
	assert.NotContains(t, string(r.Body), "Original salary note")
}
func TestScreeningChangesSubindustryWithoutChangingItsParent(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	catalogue := industryCatalogue(t, a, token)
	bank := industryNamed(t, catalogue, "FSI – Banking")
	insurance := industryNamed(t, catalogue, "FSI - Insurance")
	c := a.createCandidate(token, map[string]any{"firstName": "Synthetic Industry Screening", "industrySelections": map[string]any{"catalogueVersion": catalogue.Version, "ids": []string{bank.Id}, "primaryId": bank.Id}})
	r := a.request("POST", "/v1/candidates/"+c.Id+"/screening-calls", token, map[string]any{"version": c.Version, "industrySelections": map[string]any{"catalogueVersion": catalogue.Version, "ids": []string{insurance.Id}, "primaryId": insurance.Id}})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var result api.ScreeningCallResult
	r.JSON(&result)
	assert.Equal(t, insurance.Id, *result.Candidate.IndustrySelections.PrimaryId)
	assert.Equal(t, "Financial Services", result.Candidate.Industry.MustGet())
}
