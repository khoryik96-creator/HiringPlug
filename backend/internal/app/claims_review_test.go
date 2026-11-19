package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// completeClaim uploads a receipt and fills in everything a claim needs.
func (a *testApp) completeClaim(token, name, amount string) api.Claim {
	a.t.Helper()
	c := a.uploadReceipt(token, name, receiptBytes(name))
	return a.patchClaim(token, c, map[string]any{
		"receiptDate": daysAgo(2).Format("2006-01-02"), "amount": amount, "receiptType": "food",
		"claimType": "client_entertainment", "purpose": "Lunch with the client",
	}, http.StatusOK)
}

func refsTo(cs ...api.Claim) []map[string]any {
	out := []map[string]any{}
	for _, c := range cs {
		out = append(out, map[string]any{"id": c.Id, "version": c.Version})
	}
	return out
}

func (a *testApp) claimList(path, token string) api.ClaimPage {
	a.t.Helper()
	res := a.request(http.MethodGet, path, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var page api.ClaimPage
	res.JSON(&page)
	return page
}

func (a *testApp) claimsDone(path, token string, body map[string]any, want int) []api.Claim {
	a.t.Helper()
	res := a.request(http.MethodPost, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.ClaimList
	if want == http.StatusOK {
		res.JSON(&out)
	}
	return out.Items
}

func TestClaimsAreSubmittedDecidedAndPaid(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

	incomplete := a.uploadReceipt(recruiter, "parking.png", receiptBytes("parking"))
	lunch := a.completeClaim(recruiter, "lunch.png", "86.40")
	res := a.request(http.MethodPost, "/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(lunch, incomplete)})
	require.Equal(t, http.StatusBadRequest, res.Status, "all or none: one is incomplete")
	assert.Contains(t, string(res.Body), "parking.png: fill in the receipt date, amount, receipt type, claim type, purpose")
	assert.Equal(t, api.ClaimStatusDraft, a.claim(recruiter, lunch.Id).Status)

	lunch = a.claimsDone("/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(lunch)}, http.StatusOK)[0]
	assert.Equal(t, api.ClaimStatusSubmitted, lunch.Status)
	assert.True(t, lunch.ClaimedAt.IsSpecified() && !lunch.ClaimedAt.IsNull(), "the date claimed")
	a.patchClaim(recruiter, lunch, map[string]any{"purpose": "changed"}, http.StatusConflict)

	// Taken back before it is decided, then sent again.
	res = a.request(http.MethodPost, "/v1/claims/"+lunch.Id+"/withdraw", recruiter, map[string]any{"version": lunch.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&lunch)
	assert.Equal(t, api.ClaimStatusDraft, lunch.Status)
	lunch = a.claimsDone("/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(lunch)}, http.StatusOK)[0]

	// Only approvers see everyone's claims and decide them.
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/claims/team", recruiter, nil).Status)
	a.claimsDone("/v1/claims/review", recruiter, map[string]any{"claims": refsTo(lunch), "decision": "approve"}, http.StatusForbidden)
	team := a.claimList("/v1/claims/team", admin)
	require.Len(t, team.Items, 1, "drafts are the claimant's own")
	assert.Equal(t, lunch.Id, team.Items[0].Id)
	assert.Equal(t, "Test recruiter", team.Items[0].UserName)
	assert.Equal(t, []api.ClaimAmount{{Currency: "MYR", Amount: "86.40"}}, team.Totals)

	a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(lunch), "decision": "pay"}, http.StatusConflict)
	a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(lunch), "decision": "reject"}, http.StatusBadRequest)
	lunch = a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(lunch), "decision": "reject",
		"note": "Attach the itemised receipt"}, http.StatusOK)[0]
	assert.Equal(t, api.ClaimStatusRejected, lunch.Status)
	lunch = a.claim(recruiter, lunch.Id)
	assert.Equal(t, "Attach the itemised receipt", lunch.ReviewNote.MustGet())
	assert.Equal(t, "Test admin", lunch.ReviewedByName.MustGet())

	// Fixed and sent again; approved; paid.
	lunch = a.patchClaim(recruiter, lunch, map[string]any{"amount": "80.00"}, http.StatusOK)
	lunch = a.claimsDone("/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(lunch)}, http.StatusOK)[0]
	lunch = a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(lunch), "decision": "approve"}, http.StatusOK)[0]
	assert.Equal(t, api.ClaimStatusApproved, lunch.Status)
	res = a.request(http.MethodPost, "/v1/claims/"+lunch.Id+"/withdraw", recruiter, map[string]any{"version": lunch.Version})
	assert.Equal(t, http.StatusConflict, res.Status, "a decided claim is not withdrawn")
	stale := lunch
	lunch = a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(lunch), "decision": "pay"}, http.StatusOK)[0]
	assert.Equal(t, api.ClaimStatusPaid, lunch.Status)
	assert.True(t, lunch.PaidAt.IsSpecified() && !lunch.PaidAt.IsNull())
	a.claimsDone("/v1/claims/review", admin, map[string]any{"claims": refsTo(stale), "decision": "pay"}, http.StatusConflict)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodDelete, "/v1/claims/"+lunch.Id, recruiter, nil).Status, "a paid claim is kept")
}

func TestClaimsArePrivateToTheirClaimantAndApprovers(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleAdmin, api.RoleHiringManager)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	admin, manager := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleHiringManager].Token
	mine := a.uploadReceipt(recruiter, "grab.png", receiptBytes("grab"))
	theirs := a.uploadReceipt(sourcer, "grab.png", receiptBytes("grab")) // the same receipt, someone else's

	assert.Len(t, a.claimList("/v1/claims", recruiter).Items, 1)
	assert.Equal(t, mine.Id, a.claimList("/v1/claims", recruiter).Items[0].Id)
	for _, path := range []string{"/v1/claims/" + theirs.Id, "/v1/claims/" + theirs.Id + "/receipt"} {
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path, recruiter, nil).Status, path)
	}
	a.patchClaim(recruiter, theirs, map[string]any{"purpose": "mine now"}, http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/claims/"+theirs.Id, recruiter, nil).Status)
	a.claimsDone("/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(theirs)}, http.StatusNotFound)

	// An approver sees anyone's claim and receipt, but changes none.
	receipt := a.request(http.MethodGet, "/v1/claims/"+mine.Id+"/receipt", admin, nil)
	require.Equal(t, http.StatusOK, receipt.Status)
	assert.Equal(t, receiptBytes("grab"), receipt.Body)
	assert.Equal(t, "private, no-store", receipt.Header.Get("Cache-Control"))
	a.patchClaim(admin, a.claim(admin, mine.Id), map[string]any{"purpose": "not theirs"}, http.StatusForbidden)

	// Without claims permissions, there is nothing to see or upload.
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/claims", manager, nil).Status)
	assert.Equal(t, http.StatusForbidden, a.upload("/v1/claims", manager, "", "r.png", receiptBytes("m")).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/claims/"+mine.Id, manager, nil).Status)

	// Another company's approver sees none of them.
	other := testutil.Current.NewTenant(t, api.RoleAdmin).Users[api.RoleAdmin].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/claims/"+mine.Id, other, nil).Status)
	assert.Empty(t, a.claimList("/v1/claims/team", other).Items)
}

func TestTheClaimsDashboardAddsUpClaims(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	acme := a.createCompany(recruiter, map[string]any{"name": "Acme Berhad"})
	one := a.completeClaim(recruiter, "one.png", "10.00")
	one = a.patchClaim(recruiter, one, map[string]any{"link": map[string]any{"kind": "company", "id": acme.Id}}, http.StatusOK)
	two := a.completeClaim(recruiter, "two.png", "15.50")
	a.uploadReceipt(recruiter, "draft.png", receiptBytes("draft"))
	a.claimsDone("/v1/claims/submit", recruiter, map[string]any{"claims": refsTo(one, two)}, http.StatusOK)

	get := func(path, token string) api.ClaimSummary {
		res := a.request(http.MethodGet, path, token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var s api.ClaimSummary
		res.JSON(&s)
		return s
	}
	count := func(s api.ClaimSummary) (n int) {
		for _, f := range s.Facts {
			n += f.Count
		}
		return n
	}
	mine := get("/v1/claims/summary", recruiter)
	assert.Equal(t, 3, count(mine), "your own drafts count on your dashboard")
	team := get("/v1/claims/team/summary", admin)
	assert.Equal(t, 2, count(team), "the team's dashboard leaves drafts out")
	require.Len(t, team.People, 1)
	assert.Equal(t, "Test recruiter", team.People[0].Name)
	require.Len(t, team.Records, 1)
	assert.Equal(t, "Acme Berhad", team.Records[0].Name.MustGet())
	from := daysAgo(-1).Format("2006-01-02")
	assert.Zero(t, count(get("/v1/claims/team/summary?from="+from, admin)), "receipts dated before the period are left out")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/claims/team/summary", recruiter, nil).Status)
}

func TestAClaimTypeInUseIsHiddenNotRemoved(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	builtIn := []map[string]any{}
	for _, k := range []string{"travel", "client_entertainment", "candidate_meeting", "staff_welfare", "office", "training", "other"} {
		builtIn = append(builtIn, map[string]any{"key": k, "label": k})
	}
	put := func(options []map[string]any) *response {
		return a.request(http.MethodPut, "/v1/settings/fields/lists/claim_type", admin, map[string]any{"options": options})
	}
	res := put(append(builtIn, map[string]any{"label": "Job fair"}))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	c := a.completeClaim(recruiter, "booth.png", "300.00")
	a.patchClaim(recruiter, c, map[string]any{"claimType": "job_fair"}, http.StatusOK)
	res = put(builtIn)
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Contains(t, string(res.Body), "is still used by 1 claim")
	res = put(append(builtIn, map[string]any{"key": "job_fair", "label": "Job fair", "hidden": true}))
	require.Equal(t, http.StatusOK, res.Status, "hidden, it stays on the claim: %s", res.Body)
}
