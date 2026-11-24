package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func readCVAs(t *testing.T, a *testApp, conn *pgx.Conn, documentID, parser string) api.DocumentParseCvJob {
	t.Helper()
	job := claimAIJobFor(t, conn, documentID)
	require.Equal(t, string(api.JobKindDocumentParseCv), job.Kind)
	var asked api.DocumentParseCvJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	profile := api.ParsedProfile{
		Parser: parser, FullName: sourced("Siti Aisyah", "SITI AISYAH"), Email: nullable.NewNullNullable[api.SourcedText](),
		Phone: nullable.NewNullNullable[api.SourcedText](), LinkedinUrl: nullable.NewNullNullable[api.SourcedText](),
		Location: nullable.NewNullNullable[api.SourcedText](), Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
	}
	answerAIJob(t, a, conn, job, api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: documentID, Profile: nullable.NewNullableWithValue(profile), ErrorMessage: nullable.NewNullNullable[string](),
	})
	return asked
}

// A CV read by older reading rules says so (reviewing it reads it again);
// reading it again takes its text out of the file again (a newer reader sees
// more, such as tables, than the text saved when it came) before reading its
// details; if taking the text out fails, the text read before is read again.
func TestReadingACVAgainTakesItsTextOutOfTheFileAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	doc := withCV(t, a, aiConn, token, c.Id, "SITI AISYAH\nOrganization Role Location\nContoh Sdn Bhd Accountant Malaysia")
	readCVAs(t, a, aiConn, doc, "rules@1")
	got := a.getDocument(token, doc)
	require.NotNil(t, got.ReadOutdated)
	assert.True(t, *got.ReadOutdated, "read by older rules than this server's")

	res := a.request(http.MethodPost, "/v1/documents/"+doc+"/parse", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	assert.Equal(t, api.DocumentParseStatusQueued, a.getDocument(token, doc).ParseStatus.MustGet(), "shown as being read")
	job := claimAIJobFor(t, aiConn, doc)
	require.Equal(t, string(api.JobKindDocumentExtractText), job.Kind, "its text is taken out of the file again first")
	tables := "SITI AISYAH\nOrganization | Role | Location\nContoh Sdn Bhd | Accountant | Malaysia"
	answerAIJob(t, a, aiConn, job, api.JobKindDocumentTextExtracted, extractedText(doc, tables))
	asked := readCVAs(t, a, aiConn, doc, documents.CVReader)
	assert.Equal(t, tables, asked.Text, "then read, from the new text")
	got = a.getDocument(token, doc)
	assert.False(t, got.ReadOutdated != nil && *got.ReadOutdated, "read by this server's rules")

	// The file cannot be read now: what was read before is kept, and read again.
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/documents/"+doc+"/parse", token, nil).Status)
	job = claimAIJobFor(t, aiConn, doc)
	require.Equal(t, string(api.JobKindDocumentExtractText), job.Kind)
	answerAIJob(t, a, aiConn, job, api.JobKindDocumentTextExtracted, api.DocumentTextExtractedJob{
		DocumentId: doc, Status: api.ExtractionStatusFailed, Text: nullable.NewNullNullable[string](), Extractor: "test@1",
		PageCount: nullable.NewNullNullable[int](), ErrorMessage: nullable.NewNullableWithValue("The file could not be opened."), Warnings: []string{},
	})
	asked = readCVAs(t, a, aiConn, doc, documents.CVReader)
	assert.Equal(t, tables, asked.Text, "the text read before")
	got = a.getDocument(token, doc)
	assert.Equal(t, api.DocumentStatusExtracted, got.Status)
	assert.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet())
}
