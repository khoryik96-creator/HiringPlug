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

// A change to an offer or a placement sends it back without its salary
// (the screens read it again, asking for it, and that is counted), unless
// the change set the salary: what was just typed in. Nothing is counted.
func TestChangesSendOffersAndPlacementsBackWithoutTheirSalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	rt := recruiter.Token
	c := a.createCandidate(rt, map[string]any{"firstName": "Siti"})
	company := a.createCompany(rt, map[string]any{"name": "Contoh Berhad"})
	billing := a.createContact(rt, map[string]any{"companyId": company.Id, "firstName": "Ken", "email": "ken." + tenant.ID + "@contoh.test"})
	app := a.addToJob(rt, a.createJob(rt, map[string]any{"companyId": company.Id, "title": "Accountant"}).Id, c.Id)
	salary := func(amount string) map[string]any { return map[string]any{"amount": amount, "currency": "MYR", "period": "month"} }

	o := a.offer(http.MethodPost, "/v1/applications/"+app.Id+"/offers", rt, map[string]any{"salary": salary("9123")}, http.StatusCreated)
	require.True(t, o.Salary.IsSpecified() && !o.Salary.IsNull(), "what was just typed in")
	o = a.offer(http.MethodPatch, "/v1/offers/"+o.Id, rt, map[string]any{"version": o.Version, "terms": "Medical cover", "startDate": "2027-01-04"}, http.StatusOK)
	assert.True(t, o.Salary.IsNull(), "a change to the terms")
	o = a.offer(http.MethodPatch, "/v1/offers/"+o.Id, rt, map[string]any{"version": o.Version, "salary": salary("9200")}, http.StatusOK)
	assert.Equal(t, "9200.00", o.Salary.MustGet().Amount, "a change to the salary: what was just typed in")
	o = a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/steps", rt, map[string]any{"version": o.Version, "step": "request_approval"}, http.StatusOK)
	assert.True(t, o.Salary.IsNull(), "asked for approval")
	o = a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/decision", owner, map[string]any{"version": o.Version, "approve": true}, http.StatusOK)
	assert.True(t, o.Salary.IsNull(), "approved")
	o = a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/steps", rt, map[string]any{"version": o.Version, "step": "send"}, http.StatusOK)
	assert.True(t, o.Salary.IsNull(), "sent")

	today := time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly)
	p := a.placement(http.MethodPost, "/v1/applications/"+app.Id+"/placement", rt, map[string]any{
		"startDate": today, "money": map[string]any{"salary": salary("7345"), "feePercent": "20"},
	}, http.StatusCreated)
	require.NotNil(t, p.Money.MustGet().Salary.MustGet(), "what was just typed in")
	p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, rt, map[string]any{"version": p.Version, "poNumber": "PO-1"}, http.StatusOK)
	assert.True(t, p.Money.MustGet().Salary.IsNull(), "a change that left the money alone")
	assert.Equal(t, "17628", p.Money.MustGet().FeeAmount.MustGet()[:5], "the fee is still there")
	p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, rt, map[string]any{"version": p.Version,
		"money": map[string]any{"salary": salary("7400"), "feePercent": "20"}}, http.StatusOK)
	assert.Equal(t, "7400.00", p.Money.MustGet().Salary.MustGet().Amount, "a change to the money: what was just typed in")
	p = a.placement(http.MethodPut, "/v1/placements/"+p.Id+"/invoicing", rt, map[string]any{"contactId": billing.Id, "paymentTermsDays": 30, "dueOn": nil, "autoSend": false}, http.StatusOK)
	assert.True(t, p.Money.MustGet().Salary.IsNull(), "who is invoiced")

	seen := a.seen(owner, recruiter.ID)
	assert.NotContains(t, seen, "offer:"+c.Id)
	assert.NotContains(t, seen, "placement:"+c.Id)
}
