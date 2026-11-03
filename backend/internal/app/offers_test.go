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

func (a *testApp) offer(method, path, token string, body any, want int) api.Offer {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var o api.Offer
	res.JSON(&o)
	return o
}

func TestOffers(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleHiringManager, api.RoleSourcer)
	recruiter, manager, owner := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager], tenant.Users[api.RoleOwner]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti"}).Id)
	salary := map[string]any{"amount": "9000.00", "currency": "MYR", "period": "month"}
	step := func(o api.Offer, token, s string, want int) api.Offer {
		return a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/steps", token, map[string]any{"version": o.Version, "step": s}, want)
	}
	decide := func(o api.Offer, token string, approve bool, want int) api.Offer {
		return a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/decision", token, map[string]any{"version": o.Version, "approve": approve, "note": "ok"}, want)
	}

	o := a.offer(http.MethodPost, "/v1/applications/"+app.Id+"/offers", recruiter.Token, map[string]any{"terms": "  Medical, 18 days leave "}, http.StatusCreated)
	assert.Equal(t, api.OfferStatusDraft, o.Status)
	terms, _ := o.Terms.Get()
	assert.Equal(t, "Medical, 18 days leave", terms)
	for _, item := range a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter.Token, nil).Map()["items"].([]any) {
		assert.Equal(t, "offer", item.(map[string]any)["stage"], "making an offer moves the application to offer")
	}
	assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, "/v1/applications/"+app.Id+"/offers", recruiter.Token, map[string]any{}).Status, "one open offer")

	t.Run("recruiters ask for approval; the approver is someone else", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/offers/"+o.Id+"/steps", recruiter.Token, map[string]any{"version": o.Version, "step": "request_approval"})
		assert.Contains(t, string(res.Body), "salary", "salary and start date first")
		o = a.offer(http.MethodPatch, "/v1/offers/"+o.Id, recruiter.Token, map[string]any{"version": o.Version, "salary": salary, "startDate": "2026-12-01"}, http.StatusOK)
		step(o, recruiter.Token, "send", http.StatusUnprocessableEntity)
		o = step(o, recruiter.Token, "request_approval", http.StatusOK)
		assert.Equal(t, api.OfferStatusPendingApproval, o.Status)

		waiting := a.request(http.MethodGet, "/v1/offers?status=pending_approval", manager.Token, nil).Map()["items"].([]any)
		require.Len(t, waiting, 1)
		assert.Equal(t, "9000.00", waiting[0].(map[string]any)["salary"].(map[string]any)["amount"], "approvers see the salary they approve")
		decide(o, recruiter.Token, true, http.StatusForbidden)
		o = decide(o, manager.Token, false, http.StatusOK)
		assert.Equal(t, api.OfferStatusDraft, o.Status, "sent back")
		o = step(o, recruiter.Token, "request_approval", http.StatusOK)
		o = decide(o, manager.Token, true, http.StatusOK)
		assert.Equal(t, api.OfferStatusApproved, o.Status)
		name, _ := o.DecidedByName.Get()
		assert.Equal(t, "Test hiring_manager", name)

		// Changing what was approved needs approval again.
		changed := a.offer(http.MethodPatch, "/v1/offers/"+o.Id, recruiter.Token, map[string]any{"version": o.Version, "salary": map[string]any{"amount": "9500", "currency": "MYR", "period": "month"}}, http.StatusOK)
		assert.Equal(t, api.OfferStatusDraft, changed.Status)
		o = decide(step(changed, recruiter.Token, "request_approval", http.StatusOK), manager.Token, true, http.StatusOK)
		o = step(o, recruiter.Token, "send", http.StatusOK)
		assert.Equal(t, api.OfferStatusSent, o.Status)
		assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPatch, "/v1/offers/"+o.Id, recruiter.Token, map[string]any{"version": o.Version, "terms": "x"}).Status)
		o = step(o, recruiter.Token, "accept", http.StatusOK)
		assert.Equal(t, api.OfferStatusAccepted, o.Status)
	})

	t.Run("approvers send straight away; the salary is hidden from others", func(t *testing.T) {
		other := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"}).Id)
		o2 := a.offer(http.MethodPost, "/v1/applications/"+other.Id+"/offers", owner.Token, map[string]any{"salary": salary, "startDate": "2027-01-04"}, http.StatusCreated)
		o2 = step(o2, owner.Token, "send", http.StatusOK)
		by, _ := o2.DecidedBy.Get()
		assert.Equal(t, owner.ID, by, "sending is the owner's approval")

		list := a.request(http.MethodGet, "/v1/offers?applicationId="+other.Id, tenant.Users[api.RoleSourcer].Token, nil).Map()["items"].([]any)
		require.Len(t, list, 1)
		_, has := list[0].(map[string]any)["salary"]
		assert.False(t, has)
		assert.Equal(t, []any{"salary"}, list[0].(map[string]any)["restrictedFields"])

		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT string_agg(changes::text, ' ') FROM audit_log WHERE entity_type = 'offer' AND tenant_id = $1`, tenant.ID).Scan(&changes))
		assert.NotContains(t, changes, "9000")

		o2 = step(o2, owner.Token, "decline", http.StatusOK)
		a.offer(http.MethodPost, "/v1/applications/"+other.Id+"/offers", recruiter.Token, map[string]any{}, http.StatusCreated) // a new one after a decline
	})

	t.Run("another company sees nothing", func(t *testing.T) {
		stranger := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner]
		assert.Empty(t, a.request(http.MethodGet, "/v1/offers", stranger.Token, nil).Map()["items"])
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/offers/"+o.Id+"/steps", stranger.Token, map[string]any{"version": o.Version, "step": "withdraw"}).Status)
	})
}
