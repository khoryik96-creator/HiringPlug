package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// klDay is a day in the company's time zone (Kuala Lumpur, the default), daysAgo before today.
func klDay(t *testing.T, daysAgo int) string {
	t.Helper()
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	return time.Now().In(kl).AddDate(0, 0, -daysAgo).Format(time.DateOnly)
}

func (a *testApp) privacyOf(token, candidateID string) api.CandidatePrivacy {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/candidates/"+candidateID+"/privacy", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.CandidatePrivacy
	res.JSON(&out)
	return out
}

func (a *testApp) setConsent(token, candidateID string, body map[string]any, want int) api.Consent {
	a.t.Helper()
	res := a.request(http.MethodPut, "/v1/candidates/"+candidateID+"/consent", token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.Consent
	if want == http.StatusOK {
		res.JSON(&out)
	}
	return out
}

func (a *testApp) logRequest(token, candidateID string, body map[string]any) api.PrivacyRequest {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/candidates/"+candidateID+"/privacy-requests", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var out api.PrivacyRequest
	res.JSON(&out)
	return out
}

func TestConsentIsRecordedAndWithdrawnMeansDoNotContact(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "email": "siti." + tenant.ID + "@example.test"})

	got := a.privacyOf(recruiter, c.Id)
	assert.True(t, got.Consent.IsNull(), "none recorded yet")
	assert.False(t, got.DoNotContact)
	assert.Empty(t, got.Requests)

	given := a.setConsent(sourcer, c.Id, map[string]any{"expectedVersion": 0, "status": "given", "method": "phone", "day": klDay(t, 0), "note": " Agreed on a call. "}, http.StatusOK)
	assert.Equal(t, 1, given.Version)
	assert.Equal(t, "Agreed on a call.", given.Note.MustGet())
	a.setConsent(sourcer, c.Id, map[string]any{"expectedVersion": 0, "status": "given", "method": "phone", "day": klDay(t, 0)}, http.StatusConflict)
	a.setConsent(recruiter, c.Id, map[string]any{"expectedVersion": 1, "status": "withdrawn", "method": "email", "day": klDay(t, -1)}, http.StatusBadRequest)

	a.setConsent(recruiter, c.Id, map[string]any{"expectedVersion": 1, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	got = a.privacyOf(sourcer, c.Id)
	assert.True(t, got.DoNotContact)
	assert.Equal(t, api.ConsentStatusWithdrawn, got.Consent.MustGet().Status)

	res := a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{"to": []string{"siti." + tenant.ID + "@example.test"}, "subject": "A job", "body": "Hi", "candidateId": c.Id})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeEMAILDONOTCONTACT, res.Error().Error.Code)
	assert.Empty(t, a.emails(recruiter, c.Id), "nothing was queued")

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/privacy", tenant.Users[api.RoleViewer].Token, nil).Status)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/privacy", other, nil).Status, "another company's candidate")
	a.setConsent(other, c.Id, map[string]any{"expectedVersion": 0, "status": "given", "method": "phone", "day": klDay(t, 0)}, http.StatusNotFound)
}

func TestApplyingOnTheCareersPageIsConsentButKeepsAWithdrawal(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	var me api.Me
	a.request(http.MethodGet, "/v1/me", token, nil).JSON(&me)
	company := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Accountant", "description": "Join us."}, http.StatusCreated)
	a.adRequest(http.MethodPatch, "/v1/job-ads/"+ad.Id, token, map[string]any{"version": ad.Version, "status": "published"}, http.StatusOK)
	apply := func() {
		res := a.apply("/v1/public/careers/"+me.Tenant.CareersSlug+"/ads/"+ad.Id+"/applications",
			map[string]string{"firstName": "Nur", "email": "nur@example.com", "consent": "yes"}, "cv.pdf", minimalPDF)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	}
	apply()
	var page api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?q=nur", token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	id := page.Items[0].Id
	c := a.privacyOf(token, id).Consent.MustGet()
	assert.Equal(t, api.ConsentStatusGiven, c.Status)
	assert.Equal(t, api.ConsentMethodCareersForm, c.Method)
	assert.Equal(t, klDay(t, 0), c.Day.String())

	a.setConsent(token, id, map[string]any{"expectedVersion": c.Version, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	// Anyone can type an email into the public form: it never undoes a
	// withdrawal. The application still arrives, and the history says so,
	// for someone in the team to confirm with the candidate.
	apply()
	after := a.privacyOf(token, id).Consent.MustGet()
	assert.Equal(t, api.ConsentStatusWithdrawn, after.Status, "a withdrawal stands")
	assert.Equal(t, api.ConsentMethodEmail, after.Method)
	history := a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+id, token, nil)
	require.Equal(t, http.StatusOK, history.Status, "%s", history.Body)
	assert.Contains(t, string(history.Body), "consent.kept_withdrawn")
	a.setConsent(token, id, map[string]any{"expectedVersion": after.Version, "status": "given", "method": "phone", "day": klDay(t, 0)}, http.StatusOK)
}

func TestPrivacyRequestsAreDecidedOnTheDashboard(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
	wei := a.createCandidate(recruiter, map[string]any{"firstName": "Wei"})

	access := a.logRequest(recruiter, siti.Id, map[string]any{"kind": "access", "receivedOn": klDay(t, 30), "details": " Asked by email for a copy. "})
	assert.Equal(t, "Asked by email for a copy.", access.Details.MustGet())
	assert.Equal(t, klDay(t, 9), access.DueOn.MustGet().String(), "21 days to answer")
	assert.True(t, access.Overdue)
	assert.Equal(t, 30, access.DaysOpen)
	withdraw := a.logRequest(recruiter, wei.Id, map[string]any{"kind": "withdraw_consent", "receivedOn": klDay(t, 0)})
	assert.True(t, withdraw.DueOn.IsNull(), "no fixed time")
	res := a.request(http.MethodPost, "/v1/candidates/"+siti.Id+"/privacy-requests", recruiter, map[string]any{"kind": "access", "receivedOn": klDay(t, -2)})
	assert.Equal(t, []string{"receivedOn"}, fieldNames(t, res.Error()), "not in the future")

	decide := func(token, id string, body map[string]any, want int) api.PrivacyRequest {
		res := a.request(http.MethodPatch, "/v1/privacy-requests/"+id, token, body)
		require.Equal(t, want, res.Status, "%s", res.Body)
		var out api.PrivacyRequest
		res.JSON(&out)
		return out
	}
	decide(recruiter, withdraw.Id, map[string]any{"version": 1, "status": "done", "outcome": "x"}, http.StatusForbidden)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/privacy", recruiter, nil).Status)

	var o api.PrivacyOverview
	a.request(http.MethodGet, "/v1/privacy", owner, nil).JSON(&o)
	assert.Equal(t, api.ConsentCounts{Given: 0, Withdrawn: 0, NotRecorded: 2}, o.Consent)
	require.Len(t, o.OpenRequests, 2)
	assert.Equal(t, []string{access.Id, withdraw.Id}, []string{o.OpenRequests[0].Id, o.OpenRequests[1].Id}, "oldest first")
	assert.Equal(t, "Siti", o.OpenRequests[0].CandidateName)

	res = a.request(http.MethodPatch, "/v1/privacy-requests/"+withdraw.Id, owner, map[string]any{"version": 1, "status": "done", "outcome": "  "})
	assert.Equal(t, []string{"outcome"}, fieldNames(t, res.Error()), "say what was done")
	done := decide(owner, withdraw.Id, map[string]any{"version": 1, "status": "done", "outcome": "Marked do not contact; told them by email."}, http.StatusOK)
	assert.Equal(t, api.PrivacyRequestStatusDone, done.Status)
	assert.NotEmpty(t, done.ClosedByName.MustGet())
	decide(owner, withdraw.Id, map[string]any{"version": 1, "status": "declined", "outcome": "x"}, http.StatusConflict)
	consent := a.privacyOf(recruiter, wei.Id)
	assert.True(t, consent.DoNotContact, "closing a withdraw-consent request withdraws consent")
	assert.Equal(t, api.ConsentMethodRequest, consent.Consent.MustGet().Method)

	a.request(http.MethodGet, "/v1/privacy", owner, nil).JSON(&o)
	assert.Equal(t, api.ConsentCounts{Given: 0, Withdrawn: 1, NotRecorded: 1}, o.Consent)
	require.Len(t, o.OpenRequests, 1)
	require.Len(t, o.ClosedRequests, 1)
	require.Len(t, o.Withdrawn, 1)
	assert.Equal(t, "Wei", o.Withdrawn[0].CandidateName)

	reopened := decide(owner, withdraw.Id, map[string]any{"version": done.Version, "status": "open"}, http.StatusOK)
	assert.True(t, reopened.ClosedAt.IsNull())
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPatch, "/v1/privacy-requests/prq_01jbz6w0m0k8q9x2r3s4t5v6w7", owner,
		map[string]any{"version": 1, "status": "done", "outcome": "x"}).Status)
}

func TestACopyOfACandidatesDataHasEverythingHeld(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com", "currentTitle": "Accountant"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	withCV(t, a, aiConn, recruiter, c.Id, "Accountant at Contoh Sdn Bhd, 2019-2023")
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", recruiter, map[string]any{
		"subjectType": "candidate", "subjectId": c.Id, "kind": "screening", "body": "Strong IFRS.",
	}).Status)
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Bhd"})
	a.addToJob(recruiter, a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Finance Manager"}).Id, c.Id)
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/emails", recruiter,
		map[string]any{"to": []string{"siti@example.com"}, "subject": "Your interview", "body": "See you.", "candidateId": c.Id}).Status)
	a.setConsent(recruiter, c.Id, map[string]any{"expectedVersion": 0, "status": "given", "method": "email", "day": klDay(t, 1)}, http.StatusOK)
	a.logRequest(recruiter, c.Id, map[string]any{"kind": "access", "receivedOn": klDay(t, 0)})

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/data-export", recruiter, nil).Status)
	res := a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/data-export", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var e api.CandidateDataExport
	res.JSON(&e)
	assert.Equal(t, "Siti Aisyah", e.CandidateName)
	assert.Equal(t, "Test tenant "+t.Name(), e.Company)
	assert.Empty(t, e.NotIncluded, "an owner may see everything")
	values := map[string][]string{}
	for _, s := range e.Sections {
		for _, item := range s.Items {
			for _, f := range item.Fields {
				values[s.Title] = append(values[s.Title], f.Label+": "+f.Value)
			}
		}
	}
	assert.Contains(t, values["Profile"], "Email: siti@example.com")
	assert.Contains(t, values["Salary"], "Current salary: MYR 8500.00 a month")
	assert.Contains(t, values["CVs and documents"], "Text read from it: Accountant at Contoh Sdn Bhd, 2019-2023")
	assert.Contains(t, values["Jobs they were put forward for"], "Job: Finance Manager")
	assert.Contains(t, values["Notes"], "Note: Strong IFRS.")
	assert.Contains(t, values["Emails"], "Subject: Your interview")
	assert.Contains(t, values["Consent"], "How: Email")
	assert.Contains(t, values["Requests about their data"], "Request: A copy of their data")

	var activity api.ActivityList
	a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+c.Id, owner, nil).JSON(&activity)
	require.NotEmpty(t, activity.Items)
	assert.Equal(t, "privacy.exported", activity.Items[0].Action, "who downloaded a copy, and when")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7/data-export", owner, nil).Status)
}

func TestAMergedCandidateKeepsWithdrawnConsentAndRequests(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Wei"})
	dup := a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Dup"})
	a.setConsent(token, keep.Id, map[string]any{"expectedVersion": 0, "status": "given", "method": "phone", "day": klDay(t, 5)}, http.StatusOK)
	a.setConsent(token, dup.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 1)}, http.StatusOK)
	a.logRequest(token, dup.Id, map[string]any{"kind": "removal", "receivedOn": klDay(t, 1)})

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	got := a.privacyOf(token, keep.Id)
	assert.True(t, got.DoNotContact, "if either withdrew, do not contact")
	require.Len(t, got.Requests, 1)
	assert.Equal(t, api.PrivacyRequestKindRemoval, got.Requests[0].Kind)
}
