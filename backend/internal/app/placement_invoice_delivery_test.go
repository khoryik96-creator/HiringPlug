package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestInvoiceValidatesClientAtQueueAndDeliveryAndKeepsItsCopies(t *testing.T) {
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	first := a.createCompany(token, map[string]any{"name": "Synthetic Client One"})
	next := a.createCompany(token, map[string]any{"name": "Synthetic Client Two"})
	oldAddress := "billing-old." + tenant.ID + "@example.test"
	secondAddress := "billing-new." + tenant.ID + "@example.test"
	thirdAddress := "billing-later." + tenant.ID + "@example.test"
	ct := a.createContact(token, map[string]any{"companyId": first.Id, "firstName": "Billing", "email": oldAddress})
	job := a.createJob(token, map[string]any{"companyId": first.Id, "title": "Synthetic job", "contactId": ct.Id})
	cand := a.createCandidate(token, map[string]any{"firstName": "Synthetic Candidate"})
	appl := a.addToJob(token, job.Id, cand.Id)
	p := a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", token, map[string]any{
		"startDate": time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly),
		"money":     map[string]any{"salary": map[string]any{"amount": "10000.00", "currency": "MYR", "period": "month"}, "feePercent": "18"},
	}, http.StatusCreated)
	move := a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": ct.Version, "companyId": next.Id, "email": secondAddress})
	require.Equal(t, http.StatusOK, move.Status, "%s", move.Body)
	var moved api.Contact
	move.JSON(&moved)
	require.Equal(t, next.Id, moved.CompanyId)
	path := "/v1/placements/" + p.Id + "/invoice"
	require.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, path, token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Empty(t, mailServer.To(secondAddress))
	move = a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": moved.Version, "companyId": first.Id, "email": oldAddress})
	require.Equal(t, http.StatusOK, move.Status)
	move.JSON(&moved)
	queued := a.placement(http.MethodPost, path, token, nil, http.StatusOK)
	require.True(t, queued.Invoicing.SentAt.IsNull())
	require.Equal(t, "queued", string(*queued.Invoicing.DeliveryStatus))
	require.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, path, token, nil).Status)
	// Recipient reassignment after queuing also prevents delivery.
	move = a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": moved.Version, "companyId": next.Id})
	require.Equal(t, http.StatusOK, move.Status)
	move.JSON(&moved)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Empty(t, mailServer.To(oldAddress))
	move = a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": moved.Version, "companyId": first.Id})
	require.Equal(t, http.StatusOK, move.Status)
	move.JSON(&moved)
	retry := a.placement(http.MethodPost, path, token, nil, http.StatusOK)
	require.Equal(t, queued.InvoiceNumber, retry.InvoiceNumber)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Len(t, mailServer.To(oldAddress), 1)
	originalPDF := a.request(http.MethodGet, path+"/file?format=pdf", token, nil)
	originalWord := a.request(http.MethodGet, path+"/file?format=docx", token, nil)
	require.Equal(t, http.StatusOK, originalPDF.Status)
	require.Equal(t, http.StatusOK, originalWord.Status)
	require.Contains(t, docxText(t, originalWord.Body), oldAddress)
	move = a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": moved.Version, "email": thirdAddress})
	require.Equal(t, http.StatusOK, move.Status)
	require.Equal(t, originalPDF.Body, a.request(http.MethodGet, path+"/file?format=pdf", token, nil).Body)
	require.Equal(t, originalWord.Body, a.request(http.MethodGet, path+"/file?format=docx", token, nil).Body)
	// Older releases kept only the PDF that went out with the email.
	_, err := testutil.Current.Admin.Exec(context.Background(), "DELETE FROM placement_invoice_copy WHERE placement_id=$1", p.Id)
	require.NoError(t, err)
	require.Equal(t, originalPDF.Body, a.request(http.MethodGet, path+"/file?format=pdf", token, nil).Body)
	require.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodGet, path+"/file?format=docx", token, nil).Status)

}

func TestFailedInvoiceCanBeRetriedUnderItsOriginalNumber(t *testing.T) {
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	company := a.createCompany(token, map[string]any{"name": "Synthetic Billing Client"})
	address := "refuse." + tenant.ID + "@example.test"
	ct := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Billing", "email": address})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Synthetic job", "contactId": ct.Id})
	candidate := a.createCandidate(token, map[string]any{"firstName": "Synthetic Applicant"})
	appl := a.addToJob(token, job.Id, candidate.Id)
	p := a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", token, map[string]any{
		"startDate": time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly),
		"money":     map[string]any{"salary": map[string]any{"amount": "10000.00", "currency": "MYR", "period": "month"}, "feePercent": "18"},
	}, http.StatusCreated)
	sendPath := "/v1/placements/" + p.Id + "/invoice"
	send := a.request(http.MethodPost, sendPath, token, nil)
	require.Equal(t, http.StatusOK, send.Status, "%s", send.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		"SELECT status FROM email WHERE application_id = $1 AND $2 = ANY(to_addresses)", appl.Id, address).Scan(&status))
	require.Equal(t, "failed", status)
	invoiced := a.placements(token, "?applicationId="+appl.Id).Items[0]
	require.True(t, invoiced.Invoicing.SentAt.IsNull())
	require.False(t, invoiced.Invoicing.SendError.IsNull())
	require.Equal(t, "failed", string(*invoiced.Invoicing.DeliveryStatus))
	newAddress := "corrected." + tenant.ID + "@example.test"
	patch := a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": ct.Version, "email": newAddress})
	require.Equal(t, http.StatusOK, patch.Status)
	retry := a.placement(http.MethodPost, sendPath, token, nil, http.StatusOK)
	require.Equal(t, invoiced.InvoiceNumber, retry.InvoiceNumber)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Len(t, mailServer.To(newAddress), 1)
	sent := a.placements(token, "?applicationId="+appl.Id).Items[0]
	require.False(t, sent.Invoicing.SentAt.IsNull())
	require.True(t, sent.Invoicing.SendError.IsNull())
	require.Equal(t, "sent", string(*sent.Invoicing.DeliveryStatus))
}
