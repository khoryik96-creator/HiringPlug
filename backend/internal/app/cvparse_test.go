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
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const parsedCV = "SITI AISYAH\nsiti@example.com | +60 12-345 6789\nEXPERIENCE\nAccountant | ABC Sdn Bhd | Jan 2020 - Present\nSKILLS\nExcel, SAP\n"

// answerAIJob plays the Python worker: it queues result on the core queue and
// completes the job, in one transaction, then the Go worker runs.
func answerAIJob(t *testing.T, a *testApp, conn *pgx.Conn, job claimedJob, kind api.JobKind, result any) {
	t.Helper()
	ctx := context.Background()
	body, err := json.Marshal(result)
	require.NoError(t, err)
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT job_enqueue($1, 'core', $2, $3::jsonb, $4)`, job.TenantID, string(kind), string(body), job.RequestID)
	require.NoError(t, err)
	var completed bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT job_complete($1, 'python-test')`, job.ID).Scan(&completed))
	require.NoError(t, tx.Commit(ctx))
	runCoreJobsUntilIdle(t, a)
}

func extractedText(id, text string) api.DocumentTextExtractedJob {
	return api.DocumentTextExtractedJob{
		DocumentId: id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue(text),
		PageCount: nullable.NewNullableWithValue(1), Extractor: "test@1",
		ErrorMessage: nullable.NewNullNullable[string](), Warnings: []string{},
	}
}

func sourced(value, quote string) nullable.Nullable[api.SourcedText] {
	return nullable.NewNullableWithValue(api.SourcedText{Value: value, Quote: quote})
}

func TestCVParsingKeepsOnlyValuesFoundInTheCV(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	assert.False(t, doc.ParseStatus.IsSpecified() && !doc.ParseStatus.IsNull(), "not parsed before its text is read")

	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	// Before the text is read there is nothing to parse.
	early := a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
	assert.Equal(t, http.StatusConflict, early.Status)
	assert.Contains(t, string(early.Body), "DOCUMENT_HAS_NO_TEXT")

	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))

	// Saving the text queued parsing, with the saved text.
	parseJob := claimAIJobFor(t, aiConn, doc.Id)
	require.Equal(t, "document.parse_cv", parseJob.Kind)
	var payload api.DocumentParseCvJob
	require.NoError(t, json.Unmarshal(parseJob.Payload, &payload))
	assert.Equal(t, parsedCV, payload.Text)
	assert.Equal(t, "queued", string(a.getDocument(token, doc.Id).ParseStatus.MustGet()))

	profile := api.ParsedProfile{
		Parser: "rules@1", FullName: sourced("Siti Aisyah", "SITI AISYAH"),
		Email:        sourced("siti@example.com", "siti@example.com"),
		Phone:        sourced("+60 12-345 6789", "+60 12-345 6789"),
		LinkedinUrl:  nullable.NewNullNullable[api.SourcedText](),
		Location:     sourced("Kuala Lumpur", "Kuala Lumpur"), // not in the CV: invented
		Nationality:  sourced("MY", "SITI AISYAH"),
		NoticePeriod: sourced("forever", "SITI AISYAH"), // not a notice period
		Skills: []api.SourcedText{
			{Value: "Excel", Quote: "Excel"},
			{Value: "Python", Quote: "SAP"}, // the value is not in its quote
		},
		Experience: []api.ParsedJob{
			{
				Title: sourced("Accountant", "Accountant"), Company: sourced("ABC Sdn Bhd", "ABC Sdn Bhd"),
				Dates: sourced("Jan 2020 - Present", "Jan 2020 - Present"), StartDate: nullable.NewNullableWithValue("2020-01"),
				EndDate: nullable.NewNullNullable[string](), Current: true,
			},
			{ // an invented employer
				Title: sourced("CFO", "CFO"), Company: sourced("Big Bank", "Big Bank"),
				Dates: nullable.NewNullNullable[api.SourcedText](), StartDate: nullable.NewNullNullable[string](),
				EndDate: nullable.NewNullNullable[string](), Current: false,
			},
		},
		Education: []api.ParsedEducation{},
	}
	answerAIJob(t, a, aiConn, parseJob, api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc.Id, Profile: nullable.NewNullableWithValue(profile), ErrorMessage: nullable.NewNullNullable[string](),
	})

	got := a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet())
	p := got.ParsedProfile.MustGet()
	assert.Equal(t, "Siti Aisyah", p.FullName.MustGet().Value)
	assert.Equal(t, "+60 12-345 6789", p.Phone.MustGet().Value)
	assert.Equal(t, "MY", p.Nationality.MustGet().Value)
	assert.True(t, p.Location.IsNull(), "invented location dropped")
	assert.True(t, p.NoticePeriod.IsNull(), "malformed notice period dropped")
	assert.Equal(t, []api.SourcedText{{Value: "Excel", Quote: "Excel"}}, p.Skills)
	require.Len(t, p.Experience, 1, "invented employer dropped")
	assert.Equal(t, "ABC Sdn Bhd", p.Experience[0].Company.MustGet().Value)
	assert.Equal(t, 5, p.Dropped) // location, notice period, one skill, the invented title and company

	list := a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/documents", token, nil).Map()
	item := list["items"].([]any)[0].(map[string]any)
	assert.Equal(t, "parsed", item["parseStatus"])
	assert.NotContains(t, item, "parsedProfile", "lists leave out the parsed details")

	t.Run("parsing can be run again", func(t *testing.T) {
		again := a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
		require.Equal(t, http.StatusAccepted, again.Status, "%s", again.Body)
		var queued api.Document
		again.JSON(&queued)
		assert.Equal(t, api.DocumentParseStatusQueued, queued.ParseStatus.MustGet())
		assert.Equal(t, "document.parse_cv", claimAIJobFor(t, aiConn, doc.Id).Kind)
	})

	t.Run("viewers cannot ask for parsing", func(t *testing.T) {
		viewer := testutil.Current.NewTenant(t, api.RoleViewer).Users[api.RoleViewer].Token
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", viewer, nil).Status)
	})
}

func TestFormattedCVsAreParsedForComparison(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Wei"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "formatted", "ours.pdf", minimalPDF)
	var doc api.Document
	res.JSON(&doc)
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	// Parsed like any CV, so the compare page can show what formatting changed.
	assert.Equal(t, api.DocumentParseStatusQueued, a.getDocument(token, doc.Id).ParseStatus.MustGet())
	assert.Equal(t, "document.parse_cv", claimAIJobFor(t, aiConn, doc.Id).Kind)
}

func (a *testApp) getDocument(token, id string) api.Document {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/documents/"+id, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	return doc
}
