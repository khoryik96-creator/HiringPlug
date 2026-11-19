package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) claimStorage(token string) api.ClaimStorage {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/claims/storage", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var s api.ClaimStorage
	res.JSON(&s)
	return s
}

// storageTicket signs in to a provider as far as the callback, and returns
// the ticket Settings finishes with.
func (a *testApp) storageTicket(token, provider string) (string, url.Values) {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/claims/storage/authorize", token, map[string]any{"provider": provider})
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var auth api.ClaimStorageAuthorization
	res.JSON(&auth)
	target, err := url.Parse(auth.Url)
	require.NoError(a.t, err)
	q := target.Query()
	res = a.request(http.MethodGet, "/v1/claims/storage/callback?"+url.Values{"state": {q.Get("state")}, "code": {"synthetic-code"}}.Encode(), "", nil)
	require.Equal(a.t, http.StatusSeeOther, res.Status)
	assert.Equal(a.t, "no-store", res.Header.Get("Cache-Control"))
	back, err := url.Parse(res.Header.Get("Location"))
	require.NoError(a.t, err)
	parts := strings.SplitN(back.Fragment, "?", 2)
	require.Len(a.t, parts, 2)
	require.Equal(a.t, "/settings/claims-storage", parts[0])
	params, err := url.ParseQuery(parts[1])
	require.NoError(a.t, err)
	require.Equal(a.t, "confirm", params.Get("storage"))
	return params.Get("ticket"), q
}

func (a *testApp) connectStorage(token, provider string) api.ClaimStorage {
	a.t.Helper()
	ticket, _ := a.storageTicket(token, provider)
	res := a.request(http.MethodPost, "/v1/claims/storage/complete", token, map[string]any{"ticket": ticket})
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var s api.ClaimStorage
	res.JSON(&s)
	return s
}

func (a *testApp) submit(token string, cs ...api.Claim) []api.Claim {
	a.t.Helper()
	return a.claimsDone("/v1/claims/submit", token, map[string]any{"claims": refsTo(cs...)}, http.StatusOK)
}

// expectedPath is where a claim made by completeClaim goes.
func expectedPath(root string, c api.Claim, amount string) string {
	d := c.ReceiptDate.MustGet()
	return root + "/" + d.Format("2006-01") + "/" + c.UserName + "/Food/" + d.Format("2006-01-02") + " MYR " + amount + " (" + c.Id[len(c.Id)-6:] + ").png"
}

func TestSubmittedReceiptsAreUploadedToTheCompanyDrive(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"google_drive", "onedrive", "dropbox"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			a := newTestApp(t)
			drives := setupClaimStorage(t, a)
			tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
			recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

			s := a.connectStorage(admin, provider)
			assert.Equal(t, api.ClaimStorageStatusOk, s.Status)
			assert.Equal(t, "finance@agency.test", s.Account.MustGet())
			assert.Equal(t, provider, string(s.Provider.MustGet()))

			lunch := a.completeClaim(recruiter, "lunch.png", "86.40")
			assert.Equal(t, api.ClaimUploadStatusNone, lunch.Upload.Status, "a draft is not uploaded")
			sent := a.submit(recruiter, lunch)
			assert.Equal(t, api.ClaimUploadStatusPending, sent[0].Upload.Status)
			runCoreJobsUntilIdle(t, a, tenant.ID)

			done := a.claim(recruiter, lunch.Id)
			want := expectedPath("Claims", done, "86.40")
			assert.Equal(t, api.ClaimUploadStatusDone, done.Upload.Status, "%v", done.Upload.Error)
			assert.Equal(t, want, done.Upload.Path.MustGet())
			assert.True(t, strings.HasPrefix(done.Upload.Url.MustGet(), "https://"))
			assert.Equal(t, []string{want}, drives.paths())
			assert.Equal(t, receiptBytes("lunch.png"), drives.file(want).Data, "the receipt as uploaded")
			assert.Equal(t, 1, a.claimStorage(admin).Uploads.Done)

			// Rejected, corrected and sent again: the new receipt replaces the old.
			rejected := a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(done), "decision": "reject", "note": "Wrong amount"}, http.StatusOK)
			fixed := a.patchClaim(recruiter, rejected[0], map[string]any{"amount": "68.40"}, http.StatusOK)
			a.submit(recruiter, fixed)
			runCoreJobsUntilIdle(t, a, tenant.ID)
			assert.Equal(t, []string{expectedPath("Claims", done, "68.40")}, drives.paths(), "one file per claim")

			// Rejected again and deleted: its receipt leaves the drive too.
			again := a.claim(recruiter, lunch.Id)
			a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(again), "decision": "reject", "note": "Not claimable"}, http.StatusOK)
			require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/claims/"+lunch.Id, recruiter, nil).Status)
			runCoreJobsUntilIdle(t, a, tenant.ID)
			assert.Empty(t, drives.paths())
		})
	}
}

func TestClaimStorageIsForOwnersAndAdmins(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	setupClaimStorage(t, a)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

	for _, call := range [][2]string{{"GET", "/v1/claims/storage"}, {"POST", "/v1/claims/storage/retry"}, {"DELETE", "/v1/claims/storage"}} {
		assert.Equal(t, http.StatusForbidden, a.request(call[0], call[1], recruiter, nil).Status, call[1])
	}
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/claims/storage/authorize", recruiter, map[string]any{"provider": "dropbox"}).Status)

	// A ticket finishes only for the person who started the sign-in.
	ticket, q := a.storageTicket(admin, "onedrive")
	assert.Contains(t, q.Get("scope"), "Files.ReadWrite")
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "http://app.test/v1/claims/storage/callback", q.Get("redirect_uri"))
	other := testutil.Current.NewTenant(t, api.RoleAdmin).Users[api.RoleAdmin].Token
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/claims/storage/complete", other, map[string]any{"ticket": ticket}).Status)
	res := a.request(http.MethodGet, "/v1/claims/storage/callback?state=forged.state&code=x", "", nil)
	assert.Contains(t, res.Header.Get("Location"), "storage=failed")

	// Not connected: a submitted claim is not uploaded.
	c := a.completeClaim(recruiter, "taxi.png", "12.00")
	assert.Equal(t, api.ClaimUploadStatusNone, a.submit(recruiter, c)[0].Upload.Status)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPost, "/v1/claims/storage/retry", admin, nil).Status)

	// Tokens are kept sealed.
	a.connectStorage(admin, "onedrive")
	var sealed string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT sealed_tokens FROM claim_storage WHERE tenant_id = $1", tenant.ID).Scan(&sealed))
	assert.NotContains(t, sealed, "synthetic-")
}

func TestTheUploadFolderCanBeChosen(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	drives := setupClaimStorage(t, a)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

	s := a.claimStorage(admin)
	assert.Equal(t, "Claims", s.RootFolder)
	bad := a.request(http.MethodPut, "/v1/claims/storage", admin, map[string]any{"rootFolder": " / .. ", "version": s.Version})
	assert.Equal(t, http.StatusBadRequest, bad.Status)
	res := a.request(http.MethodPut, "/v1/claims/storage", admin, map[string]any{"rootFolder": " Finance / Claims: 2026 ", "version": s.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&s)
	assert.Equal(t, "Finance/Claims- 2026", s.RootFolder)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, "/v1/claims/storage", admin, map[string]any{"rootFolder": "Old", "version": s.Version - 1}).Status)

	a.connectStorage(admin, "google_drive")
	c := a.completeClaim(recruiter, "dinner.png", "120.00")
	a.submit(recruiter, c)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, []string{expectedPath("Finance/Claims- 2026", a.claim(recruiter, c.Id), "120.00")}, drives.paths())
}

func TestFailedUploadsAreKeptAndTriedAgain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	drives := setupClaimStorage(t, a)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	a.connectStorage(admin, "dropbox")

	// A busy provider: tried again later, by the queue.
	drives.mu.Lock()
	drives.fail, drives.failTimes = 503, 1
	drives.mu.Unlock()
	busy := a.completeClaim(recruiter, "busy.png", "10.00")
	a.submit(recruiter, busy)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID)
	assert.Equal(t, api.ClaimUploadStatusDone, a.claim(recruiter, busy.Id).Upload.Status)

	// Access withdrawn: the storage asks to be connected again, the upload
	// waits as failed, and connecting again uploads it.
	drives.mu.Lock()
	drives.fail, drives.failTimes = 401, 1
	drives.mu.Unlock()
	refused := a.completeClaim(recruiter, "refused.png", "11.00")
	a.submit(recruiter, refused)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.claim(recruiter, refused.Id)
	assert.Equal(t, api.ClaimUploadStatusFailed, got.Upload.Status)
	assert.Contains(t, got.Upload.Error.MustGet(), "connect the account again")
	s := a.claimStorage(admin)
	assert.Equal(t, api.ClaimStorageStatusReconnect, s.Status)
	assert.Equal(t, 1, s.Uploads.Failed)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPost, "/v1/claims/storage/retry", admin, nil).Status, "reconnect first")

	a.connectStorage(admin, "dropbox")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.ClaimUploadStatusDone, a.claim(recruiter, refused.Id).Upload.Status)
	assert.Len(t, drives.paths(), 2)

	// A refusal that is not about access: failed, and Try again uploads it.
	drives.mu.Lock()
	drives.fail, drives.failTimes = 400, 1
	drives.mu.Unlock()
	odd := a.completeClaim(recruiter, "odd.png", "12.00")
	a.submit(recruiter, odd)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.ClaimUploadStatusFailed, a.claim(recruiter, odd.Id).Upload.Status)
	res := a.request(http.MethodPost, "/v1/claims/storage/retry", admin, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var queued api.ClaimUploadsQueued
	res.JSON(&queued)
	assert.Equal(t, 1, queued.Queued)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.ClaimUploadStatusDone, a.claim(recruiter, odd.Id).Upload.Status)
}

func TestStorageTokensAreRefreshedAndKept(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	drives := setupClaimStorage(t, a)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

	drives.mu.Lock()
	drives.noRefresh = true
	drives.mu.Unlock()
	ticket, _ := a.storageTicket(admin, "google_drive")
	res := a.request(http.MethodPost, "/v1/claims/storage/complete", admin, map[string]any{"ticket": ticket})
	assert.Equal(t, http.StatusBadRequest, res.Status, "no lasting access: not connected")
	assert.Contains(t, string(res.Body), "offline access")

	drives.mu.Lock()
	drives.noRefresh, drives.expiresIn = false, 60 // expiring soon: refreshed before each use
	drives.mu.Unlock()
	a.connectStorage(admin, "google_drive")
	for _, name := range []string{"one.png", "two.png"} {
		a.submit(recruiter, a.completeClaim(recruiter, name, "5.00"))
		runCoreJobsUntilIdle(t, a, tenant.ID)
	}
	assert.Len(t, drives.paths(), 2)
	drives.mu.Lock()
	assert.Equal(t, 2, drives.refreshes, "the refresh token was kept for the second refresh")
	drives.refused = true
	drives.mu.Unlock()

	third := a.completeClaim(recruiter, "three.png", "5.00")
	a.submit(recruiter, third)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.ClaimUploadStatusFailed, a.claim(recruiter, third.Id).Upload.Status)
	assert.Equal(t, api.ClaimStorageStatusReconnect, a.claimStorage(admin).Status, "a refresh token refused: connect again")
}
