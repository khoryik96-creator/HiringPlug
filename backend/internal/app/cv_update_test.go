package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func cvJob(title, company, dates, start string, current bool) api.ParsedJob {
	end := nullable.NewNullNullable[string]()
	if !current {
		end = nullable.NewNullableWithValue(start[:4])
	}
	return api.ParsedJob{
		Title: sourced(title, title), Company: sourced(company, company), Dates: sourced(dates, dates),
		StartDate: nullable.NewNullableWithValue(start), EndDate: end, Current: current,
	}
}

func cvProfile(location string, jobs ...api.ParsedJob) api.ParsedProfile {
	null := nullable.NewNullNullable[api.SourcedText]()
	return api.ParsedProfile{
		Parser: "rules@1", FullName: sourced("Siti Aisyah", "SITI AISYAH"), Email: null, Phone: null, LinkedinUrl: null,
		Location: sourced(location, location), Nationality: null, ResidenceCountry: null, Industry: null, NoticePeriod: null,
		Skills: []api.SourcedText{{Value: "IFRS", Quote: "IFRS"}}, Experience: jobs, Education: []api.ParsedEducation{},
	}
}

// readNewCV plays the AI worker reading an uploaded CV: its text, then its details.
func readNewCV(t *testing.T, a *testApp, conn *pgx.Conn, documentID, text string, profile api.ParsedProfile) {
	t.Helper()
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, documentID), api.JobKindDocumentTextExtracted, extractedText(documentID, text))
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, documentID), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: documentID, Profile: nullable.NewNullableWithValue(profile), ErrorMessage: nullable.NewNullNullable[string](),
	})
}

func TestANewCVBringsTheProfileUpToDate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{
		"firstName": "Siti", "lastName": "Aisyah", "currentTitle": "Accountant", "currentCompany": "ABC Sdn Bhd", "location": "Ipoh",
		"skills": []string{"Excel"},
	})
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	const newCV = "SITI AISYAH\nKuala Lumpur\nFinance Manager | XYZ Berhad | Mar 2025 - Present\nAccountant | ABC Sdn Bhd | Jan 2020 - Feb 2025\nIFRS\n"
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv-2025.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	readNewCV(t, a, conn, doc.Id, newCV, cvProfile("Kuala Lumpur",
		cvJob("Finance Manager", "XYZ Berhad", "Mar 2025 - Present", "2025-03", true),
		cvJob("Accountant", "ABC Sdn Bhd", "Jan 2020 - Feb 2025", "2020-01", false)))

	got := getCandidate(t, a, token, c.Id)
	assert.Equal(t, "Finance Manager", got.CurrentTitle.MustGet(), "where they work now replaces the old")
	assert.Equal(t, "XYZ Berhad", got.CurrentCompany.MustGet())
	assert.Equal(t, "Kuala Lumpur", got.Location.MustGet())
	assert.Equal(t, "Siti", got.FirstName, "the name is left as it is")
	assert.ElementsMatch(t, []string{"Excel", "IFRS"}, got.Skills, "new skills are added")
	require.Len(t, got.Experience, 2, "the CV's work history, longer than the profile's")
	assert.Equal(t, "XYZ Berhad", got.Experience[0].Company.MustGet())
	var changes map[string]any
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT changes FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 AND action = 'candidate.updated_from_cv'`, tenant.ID, c.Id).Scan(&changes))
	assert.Equal(t, doc.Id, changes["documentId"])

	t.Run("an older CV read again changes nothing", func(t *testing.T) {
		older := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv-2019.pdf", append([]byte{}, append(minimalPDF, "% old\n"...)...))
		require.Equal(t, http.StatusCreated, older.Status, "%s", older.Body)
		var old api.Document
		older.JSON(&old)
		// A newer upload, but its CV is older than the profile's history: it only fills what is empty.
		readNewCV(t, a, conn, old.Id, "SITI AISYAH\nPenang\nAccountant | ABC Sdn Bhd | Jan 2020 - Present\n",
			cvProfile("Penang", cvJob("Accountant", "ABC Sdn Bhd", "Jan 2020 - Present", "2020-01", true)))
		after := getCandidate(t, a, token, c.Id)
		assert.Equal(t, "Finance Manager", after.CurrentTitle.MustGet())
		assert.Equal(t, "Kuala Lumpur", after.Location.MustGet())
		assert.Len(t, after.Experience, 2)

		// The first CV is no longer the newest: reading it again changes nothing either.
		edited := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": after.Version, "currentTitle": "Head of Finance"})
		require.Equal(t, http.StatusOK, edited.Status, "%s", edited.Body)
		again := a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
		require.Equal(t, http.StatusAccepted, again.Status, "%s", again.Body)
		answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
			DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](),
			Profile: nullable.NewNullableWithValue(cvProfile("Kuala Lumpur", cvJob("Finance Manager", "XYZ Berhad", "Mar 2025 - Present", "2025-03", true))),
		})
		assert.Equal(t, "Head of Finance", getCandidate(t, a, token, c.Id).CurrentTitle.MustGet(), "a hand edit stays")
	})
}

func getCandidate(t *testing.T, a *testApp, token, id string) api.Candidate {
	t.Helper()
	res := a.request(http.MethodGet, "/v1/candidates/"+id, token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var c api.Candidate
	res.JSON(&c)
	return c
}
