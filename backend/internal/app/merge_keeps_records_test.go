package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Merging two candidates who are on the same job never deletes an
// application that carries interviews, offers or a placement.
func TestMergingKeepsThePlacementOnASharedJob(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	token := tenant.Users[api.RoleAdmin].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "tags": []string{"relocate"}, "skills": []string{"Audit"}})
	dup := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aishah", "tags": []string{"Relocate", "shortlist"}, "skills": []string{"SAP", "audit"}})
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	a.addToJob(token, job.Id, keep.Id) // the kept record: only applied
	placedApp := a.addToJob(token, job.Id, dup.Id)
	placement := a.placement(http.MethodPost, "/v1/applications/"+placedApp.Id+"/placement", token, map[string]any{
		"startDate": "2026-12-01", "money": map[string]any{"feeAmount": "20000.00", "feeCurrency": "MYR"},
	}, http.StatusCreated)

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	list := a.placements(token, "")
	require.Len(t, list.Items, 1, "the placement is still there")
	assert.Equal(t, placement.Id, list.Items[0].Id)
	assert.Equal(t, keep.Id, list.Items[0].CandidateId, "now the kept candidate's")
	assert.Len(t, *list.FeeTotals, 1, "and still billed")
	apps := a.request(http.MethodGet, "/v1/candidates/"+keep.Id+"/applications", token, nil).Map()["items"].([]any)
	require.Len(t, apps, 1, "on the job once")
	assert.Equal(t, "placed", apps[0].(map[string]any)["stage"])

	merged := a.request(http.MethodGet, "/v1/candidates/"+keep.Id, token, nil).Map()
	assert.Equal(t, []any{"relocate", "shortlist"}, merged["tags"], "tags joined, the kept spelling first")
	assert.Equal(t, []any{"Audit", "SAP"}, merged["skills"])
}

func TestMergingIsRefusedWhenBothApplicationsCarryRecords(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	admin := tenant.Users[api.RoleAdmin]
	token := admin.Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	dup := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aishah"})
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	keepApp, dupApp := a.addToJob(token, job.Id, keep.Id), a.addToJob(token, job.Id, dup.Id)
	a.interview(http.MethodPost, "/v1/applications/"+keepApp.Id+"/interviews", token, map[string]any{
		"title": "First interview", "startsAt": time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute), "durationMinutes": 60, "mode": "video",
		"location": "https://meet.example/abc", "interviewerIds": []string{admin.ID},
	}, http.StatusCreated)
	a.placement(http.MethodPost, "/v1/applications/"+dupApp.Id+"/placement", token, map[string]any{"startDate": "2026-12-01"}, http.StatusCreated)

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeAPPLICATIONSOVERLAP, res.Error().Error.Code)
	assert.Contains(t, res.Error().Error.Message, "Auditor")
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+dup.Id, token, nil).Status, "nothing was merged or deleted")
	assert.Len(t, a.placements(token, "").Items, 1)
}

// What was made for the duplicate follows it: a booking link already sent
// still opens, and its CV formatting runs and import files point to the
// candidate kept.
func TestMergingMovesBookingLinksAndFormattingRuns(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	token := tenant.Users[api.RoleAdmin].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	dup := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aishah", "email": "siti." + tenant.ID + "@mail.test"})
	var hours []map[string]any
	for d := 1; d <= 7; d++ {
		hours = append(hours, map[string]any{"day": d, "start": "08:00", "end": "18:00"})
	}
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/scheduling/me", token, map[string]any{
		"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 3,
	}).Status)
	link := a.bookingLink(token, map[string]any{"candidateId": dup.Id, "title": "Call", "durationMinutes": 30})
	url, err := link.Url.Get()
	require.NoError(t, err)
	public := tokenInURL.FindStringSubmatch(url)[1]
	ctx := context.Background()
	_, err = testutil.Current.Admin.Exec(ctx,
		`INSERT INTO cv_import (id, tenant_id, source, created_by) VALUES ('cvi_'||substr($2, 6), $1, 'linkedin', $3)`, tenant.ID, dup.Id, tenant.Users[api.RoleAdmin].ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx,
		`INSERT INTO cv_import_item (tenant_id, import_id, file_name, candidate_id, status) VALUES ($1, 'cvi_'||substr($2, 6), 'cv.pdf', $2, 'created')`, tenant.ID, dup.Id)
	require.NoError(t, err)

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	page := a.publicBooking(public)
	assert.NotEmpty(t, page.Slots, "the link sent before the merge still offers times")
	slot := page.Slots[0]
	booked := a.request(http.MethodPost, "/v1/public/bookings/"+public, "", map[string]any{"startsAt": slot})
	require.Equal(t, http.StatusOK, booked.Status, "and can be booked: %s", booked.Body)
	var links struct{ Items []api.BookingLink }
	a.request(http.MethodGet, "/v1/booking-links?candidateId="+keep.Id, token, nil).JSON(&links)
	assert.Len(t, links.Items, 1, "listed on the candidate kept")
	var item string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT candidate_id FROM cv_import_item WHERE tenant_id = $1`, tenant.ID).Scan(&item))
	assert.Equal(t, keep.Id, item, "the import file points to the candidate kept")
}
