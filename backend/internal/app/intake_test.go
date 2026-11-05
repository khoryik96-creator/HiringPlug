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

func (a *testApp) cvImport(token, id string) api.CvImport {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/cv-imports/"+id, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var imp api.CvImport
	res.JSON(&imp)
	return imp
}

// readImported plays the AI worker for an imported file: its text, then the
// details read from it (nil: they could not be read).
func readImported(t *testing.T, a *testApp, conn *pgx.Conn, itemID, text string, profile *api.ParsedProfile) {
	t.Helper()
	var docID string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT document_id FROM cv_import_item WHERE id = $1`, itemID).Scan(&docID))
	if text == "" { // no text: reading ends there
		failed := extractedText(docID, "")
		failed.Status, failed.Text, failed.ErrorMessage = api.ExtractionStatusFailed, nullable.NewNullNullable[string](), nullable.NewNullableWithValue("damaged")
		answerAIJob(t, a, conn, claimAIJobFor(t, conn, docID), api.JobKindDocumentTextExtracted, failed)
		return
	}
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, docID), api.JobKindDocumentTextExtracted, extractedText(docID, text))
	result := api.DocumentCvParsedJob{DocumentId: docID, ErrorMessage: nullable.NewNullableWithValue("could not read"), Profile: nullable.NewNullNullable[api.ParsedProfile]()}
	if profile != nil {
		result.ErrorMessage, result.Profile = nullable.NewNullNullable[string](), nullable.NewNullableWithValue(*profile)
	}
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, docID), api.JobKindDocumentCvParsed, result)
}

// cvOf is a CV's text and what reading it found (each value quoting it).
func cvOf(name, email, linkedin string) (string, *api.ParsedProfile) {
	return name + "\n" + email + " | " + linkedin, &api.ParsedProfile{
		Parser: "rules@1", FullName: sourced(name, name), Email: sourced(email, email), LinkedinUrl: sourced(linkedin, linkedin),
		Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
	}
}

func TestImportingCVsAndLinkedInPDFs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	existing := a.createCandidate(token, map[string]any{"firstName": "Bala", "email": "bala@example.com"})

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/cv-imports", tenant.Users[api.RoleViewer].Token, map[string]any{"source": "linkedin"}).Status)
	res := a.request(http.MethodPost, "/v1/cv-imports", token, map[string]any{"source": "roadshow"})
	assert.Equal(t, []string{"source"}, fieldNames(t, res.Error()))
	res = a.request(http.MethodPost, "/v1/cv-imports", token, map[string]any{"source": "linkedin"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var imp api.CvImport
	res.JSON(&imp)

	add := func(name string, data []byte) api.CvImportItem {
		res := a.upload("/v1/cv-imports/"+imp.Id+"/files", token, "", name, data)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var it api.CvImportItem
		res.JSON(&it)
		return it
	}
	fresh := add("Profile_Siti-Aisyah.pdf", minimalPDF)
	assert.Equal(t, api.CvImportItemStatusReading, fresh.Status)
	assert.Equal(t, "Profile Siti Aisyah", fresh.CandidateName.MustGet(), "the answer names the new candidate too")
	dup := add("Bala.pdf", minimalPDF)
	unreadable := add("scan.pdf", minimalPDF)
	notCV := add("notes.txt", []byte("just some notes"))
	assert.Equal(t, api.CvImportItemStatusFailed, notCV.Status)
	assert.True(t, notCV.CandidateId.IsNull(), "a file that is not a CV makes no candidate")

	placeholder := a.cvImport(token, imp.Id).Items[0]
	assert.Equal(t, "Profile Siti Aisyah", placeholder.CandidateName.MustGet(), "named after the file while it is read")

	text, profile := cvOf("Siti Aisyah binti Ahmad", "siti@example.com", "https://my.linkedin.com/in/siti-aisyah")
	readImported(t, a, conn, fresh.Id, text, profile)
	text, profile = cvOf("Bala Kumar", "BALA@example.com", "linkedin.com/in/bala")
	readImported(t, a, conn, dup.Id, text, profile)
	readImported(t, a, conn, unreadable.Id, "", nil)

	got := a.cvImport(token, imp.Id)
	assert.Equal(t, api.CvImportCounts{Reading: 0, Created: 1, Added: 1, Unread: 1, Failed: 1}, got.Counts)
	byID := map[string]api.CvImportItem{}
	for _, it := range got.Items {
		byID[it.Id] = it
	}

	created := byID[fresh.Id]
	assert.Equal(t, api.CvImportItemStatusCreated, created.Status)
	var c api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+created.CandidateId.MustGet(), token, nil).JSON(&c)
	assert.Equal(t, "Siti", c.FirstName)
	assert.Equal(t, "Aisyah binti Ahmad", c.LastName)
	assert.EqualValues(t, "siti@example.com", c.Email.MustGet())
	assert.Equal(t, "https://www.linkedin.com/in/siti-aisyah", c.LinkedinUrl.MustGet(), "the LinkedIn link, tidied")
	assert.Equal(t, api.CandidateSource("linkedin"), c.Source.MustGet())
	assert.Equal(t, tenant.Users[api.RoleRecruiter].ID, c.OwnerId.MustGet(), "owned by who imported it")

	added := byID[dup.Id]
	assert.Equal(t, api.CvImportItemStatusAdded, added.Status, "one candidate per email")
	assert.Equal(t, existing.Id, added.CandidateId.MustGet())
	var docs api.DocumentList
	a.request(http.MethodGet, "/v1/candidates/"+existing.Id+"/documents", token, nil).JSON(&docs)
	require.Len(t, docs.Items, 1, "the CV joined the candidate who has its email")
	assert.Equal(t, api.DocumentSource("linkedin"), docs.Items[0].Source)
	var bala api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+existing.Id, token, nil).JSON(&bala)
	assert.Equal(t, "Bala", bala.FirstName, "their own name stays")
	assert.Equal(t, "https://www.linkedin.com/in/bala", bala.LinkedinUrl.MustGet(), "empty fields are filled from the CV")

	assert.Equal(t, api.CvImportItemStatusUnread, byID[unreadable.Id].Status)
	assert.Equal(t, "scan", byID[unreadable.Id].CandidateName.MustGet(), "kept, named after the file")

	t.Run("imports are the importer's own", func(t *testing.T) {
		var list api.CvImportList
		a.request(http.MethodGet, "/v1/cv-imports", token, nil).JSON(&list)
		require.Len(t, list.Items, 1)
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/cv-imports/"+imp.Id, other, nil).Status)
		res := a.upload("/v1/cv-imports/"+imp.Id+"/files", other, "", "x.pdf", minimalPDF)
		assert.Equal(t, http.StatusNotFound, res.Status)
	})
}
