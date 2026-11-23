package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var shortlistTokenInURL = regexp.MustCompile(`/shortlist/([A-Za-z0-9_-]{43})$`)

// shortlistJob is a job with a hiring contact, and candidates on it at
// submitted, each with a formatted CV.
type shortlistJob struct {
	job        api.JobOrder
	contact    string // the hiring contact's email
	apps       map[string]api.Application
	cvs        map[string]string // first name -> formatted CV
	candidates map[string]api.Candidate
}

func (a *testApp) shortlistJob(token, tenantID string, people map[string]map[string]any) shortlistJob {
	a.t.Helper()
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	email := "aisyah." + strings.ToLower(tenantID) + "@maju.test"
	contact := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman", "email": email})
	out := shortlistJob{
		job:     a.createJob(token, map[string]any{"companyId": company.Id, "contactId": contact.Id, "title": "Finance Manager"}),
		contact: email, apps: map[string]api.Application{}, cvs: map[string]string{}, candidates: map[string]api.Candidate{},
	}
	for first, cv := range people {
		c := a.createCandidate(token, map[string]any{
			"firstName": first, "lastName": "Tan", "email": strings.ToLower(first) + "." + tenantID + "@mail.test", "phone": "+60123456789",
		})
		app := a.addToJob(token, out.job.Id, c.Id)
		res := a.move(token, app, map[string]any{"stage": "submitted"})
		require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&app)
		made, _ := a.formatCv(token, c.Id, cv, http.StatusCreated)
		out.apps[first], out.cvs[first], out.candidates[first] = app, made.DocumentId, c
	}
	return out
}

func (a *testApp) makeShortlist(token, jobID string, body map[string]any, want int) (api.ClientShortlistCreated, string) {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/jobs/"+jobID+"/client-shortlists", token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.ClientShortlistCreated
	if want != http.StatusCreated {
		return out, ""
	}
	res.JSON(&out)
	m := shortlistTokenInURL.FindStringSubmatch(out.Url)
	require.NotNil(a.t, m, out.Url)
	return out, m[1]
}

func (a *testApp) clientPage(token string) (api.PublicClientShortlist, string) {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/public/client-shortlists/"+token, "", nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.PublicClientShortlist
	res.JSON(&out)
	return out, string(res.Body)
}

func (a *testApp) clientAnswer(token, itemID string, body map[string]any) *response {
	a.t.Helper()
	return a.request(http.MethodPost, "/v1/public/client-shortlists/"+token+"/candidates/"+itemID+"/answer", "", body)
}

func (a *testApp) activityOf(token, entityType, id string) []string {
	a.t.Helper()
	var list api.ActivityList
	a.request(http.MethodGet, "/v1/activity?entityType="+entityType+"&entityId="+id, token, nil).JSON(&list)
	var out []string
	for _, e := range list.Items {
		out = append(out, e.Action)
	}
	return out
}

func countIn(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

func TestClientShortlistLink(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {"blind": true}, "Wei": {}})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+sj.candidates["Siti"].Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8513", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/client-shortlists", tenant.Users[api.RoleViewer].Token, nil).Status,
		"viewers do not share with clients")
	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{
		"candidates": []any{
			map[string]any{"applicationId": sj.apps["Siti"].Id, "documentId": sj.cvs["Siti"], "note": " Strong on month-end close. "},
			map[string]any{"applicationId": sj.apps["Wei"].Id, "documentId": sj.cvs["Wei"]},
		},
		"message": "Two for the finance role.", "expiresInDays": 7, "emailHiringContact": true,
	}, http.StatusCreated)
	s := created.Shortlist
	assert.Equal(t, api.ClientShortlistStatusOpen, s.Status)
	assert.Equal(t, sj.contact, s.EmailedTo.MustGet())
	assert.True(t, s.OpenedAt.IsNull(), "not opened yet")
	require.Len(t, s.Candidates, 2)
	siti, wei := s.Candidates[0], s.Candidates[1]
	assert.Regexp(t, `^Candidate C-[0-9A-Z]{6}$`, siti.Label, "a blind CV: the client sees a reference")
	assert.True(t, siti.Blind)
	assert.Equal(t, "Siti Tan", siti.CandidateName, "the recruiter sees the name")
	assert.Equal(t, "Wei Tan", wei.Label)
	assert.Equal(t, "Strong on month-end close.", siti.Note.MustGet())

	// The hiring contact gets the link from the recruiter.
	runCoreJobsUntilIdle(t, a, tenant.ID)
	msgs := mailServer.To(sj.contact)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Data, "Finance Manager")
	assert.Contains(t, msgs[0].Data, "works until "+s.ExpiresAt.In(a.deps.Config.TimeZone).Format("2 January 2006"), "the agency's day, not UTC's")

	page, raw := a.clientPage(token)
	assert.Equal(t, "Finance Manager", page.JobTitle)
	assert.Equal(t, "Test recruiter", page.RecruiterName.MustGet())
	assert.Equal(t, "Two for the finance role.", page.Message.MustGet())
	require.Len(t, page.Candidates, 2)
	assert.Equal(t, siti.Label, page.Candidates[0].Label)
	assert.Equal(t, siti.Label+" CV.docx", page.Candidates[0].CvFileName.MustGet(), "as the download is named: no PDF is made in tests")
	for _, hidden := range []string{"Siti", "siti.", "+60123456789", "123456789", "8513", sj.candidates["Siti"].Id, sj.apps["Siti"].Id} {
		assert.NotContains(t, raw, hidden, "the client sees only the label, the note and the CV")
	}
	a.clientPage(token)
	var list api.ClientShortlistList
	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.False(t, list.Items[0].OpenedAt.IsNull(), "the recruiter sees it was opened")

	// The CV, named by the label (no PDF made in tests: the Word file).
	res := a.request(http.MethodGet, "/v1/public/client-shortlists/"+token+"/candidates/"+siti.Id+"/cv", "", nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Contains(t, res.Header.Get("Content-Disposition"), siti.Label+" CV.docx")
	text := docxText(t, res.Body)
	assert.NotContains(t, text, "Siti")
	assert.NotContains(t, text, "8513")

	// Approve: on to interview. Reject: closed, client rejected, the comment as the note.
	res = a.clientAnswer(token, siti.Id, map[string]any{"decision": "approved", "comment": " Let's meet her. "})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&page)
	assert.Equal(t, api.ClientShortlistDecisionApproved, page.Candidates[0].Decision.MustGet())
	res = a.clientAnswer(token, siti.Id, map[string]any{"decision": "rejected"})
	require.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, api.ErrorCodeSHORTLISTALREADYANSWERED, res.Error().Error.Code)
	res = a.clientAnswer(token, wei.Id, map[string]any{"decision": "rejected", "comment": "Too junior for us."})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusBadRequest, a.clientAnswer(token, wei.Id, map[string]any{"decision": "maybe"}).Status)

	var apps api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/applications", recruiter, nil).JSON(&apps)
	stages := map[string]api.Application{}
	for _, app := range apps.Items {
		stages[app.CandidateId] = app
	}
	assert.Equal(t, api.StageInterview, stages[sj.candidates["Siti"].Id].Stage)
	rejected := stages[sj.candidates["Wei"].Id]
	assert.Equal(t, api.StageRejected, rejected.Stage)
	assert.Equal(t, api.CloseReasonClientRejected, rejected.Reason.MustGet())
	assert.Equal(t, "Too junior for us.", rejected.ReasonNote.MustGet())

	a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/client-shortlists", recruiter, nil).JSON(&list)
	got := list.Items[0].Candidates
	assert.Equal(t, "Let's meet her.", got[0].Comment.MustGet())
	assert.Equal(t, api.ClientShortlistDecisionRejected, got[1].Decision.MustGet())

	// On the job's and the candidate's activity; the first visit only once.
	onJob := a.activityOf(recruiter, "job", sj.job.Id)
	assert.Equal(t, 1, countIn(onJob, "client_shortlist.created"))
	assert.Equal(t, 1, countIn(onJob, "client_shortlist.opened"), "the first visit only")
	assert.Equal(t, 2, countIn(onJob, "client_shortlist.answered"))
	onSiti := a.activityOf(recruiter, "candidate", sj.candidates["Siti"].Id)
	assert.Contains(t, onSiti, "client_shortlist.shared")
	assert.Contains(t, onSiti, "client_shortlist.answered")

	// Revoked: the client sees that it no longer works.
	res = a.request(http.MethodPost, "/v1/client-shortlists/"+s.Id+"/revoke", recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	page, _ = a.clientPage(token)
	assert.Equal(t, api.ClientShortlistStatusRevoked, page.Status)
	assert.Empty(t, page.Candidates)
	assert.True(t, page.Message.IsNull())
	assert.Equal(t, http.StatusConflict, a.request(http.MethodGet, "/v1/public/client-shortlists/"+token+"/candidates/"+siti.Id+"/cv", "", nil).Status)
	assert.Contains(t, a.activityOf(recruiter, "job", sj.job.Id), "client_shortlist.revoked")

	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/public/client-shortlists/"+strings.Repeat("A", 43), "", nil).Status)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/"+sj.job.Id+"/client-shortlists", other.Users[api.RoleOwner].Token, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/client-shortlists/"+s.Id+"/revoke", other.Users[api.RoleOwner].Token, nil).Status)
}
