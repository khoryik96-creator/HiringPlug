package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// BUG-004: a CV stayed "Reading details... 413s" forever when the AI
// worker's job for it failed for good: nothing told the document.

const (
	keptWarning = "The CV could not be read again, so the details read before are kept. Read it again later."
	noDetails   = "The details could not be read from this CV. Read it again, or fill them in by hand."
	fileMissing = "The file was not found on the server, so its text could not be read. Upload it again."
	readingLost = "Reading this file stopped before it finished (the server was restarted or restored). Read it again."
)

// failAIJob plays the Python worker failing a claimed job for good: its last
// attempt failing (job_fail), or given up (job_give_up).
func failAIJob(t *testing.T, conn *pgx.Conn, job claimedJob, giveUp bool, reason string) {
	t.Helper()
	ctx := context.Background()
	fn := "job_give_up"
	if !giveUp {
		fn = "job_fail"
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET max_attempts = attempts WHERE id = $1`, job.ID)
		require.NoError(t, err)
	}
	var status string
	require.NoError(t, conn.QueryRow(ctx, `SELECT `+fn+`($1, 'python-test', $2)`, job.ID, reason).Scan(&status))
	require.Equal(t, "dead", status)
}

// activeReadingJobs counts a document's reading jobs waiting or running.
func activeReadingJobs(t *testing.T, documentID string) int {
	t.Helper()
	var n int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT count(*) FROM job
		WHERE status IN ('pending', 'running') AND kind IN ('document.extract_text', 'document.parse_cv') AND payload->>'documentId' = $1`,
		documentID).Scan(&n))
	return n
}

// readParsedCV uploads a PDF CV and reads it (text, then details) as the workers would.
func readParsedCV(t *testing.T, a *testApp, conn *pgx.Conn, token, firstName string) api.Document {
	t.Helper()
	c := a.createCandidate(token, map[string]any{"firstName": firstName})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](),
		Profile: nullable.NewNullableWithValue(api.ParsedProfile{
			Parser: "rules@1", FullName: sourced("Siti Aisyah", "SITI AISYAH"), Email: sourced("siti@example.com", "siti@example.com"),
			Phone: nullable.NewNullNullable[api.SourcedText](), LinkedinUrl: nullable.NewNullNullable[api.SourcedText](),
			Location: nullable.NewNullNullable[api.SourcedText](), Nationality: nullable.NewNullNullable[api.SourcedText](),
			ResidenceCountry: nullable.NewNullNullable[api.SourcedText](), Industry: nullable.NewNullNullable[api.SourcedText](),
			NoticePeriod: nullable.NewNullNullable[api.SourcedText](),
			Skills:       []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
		}),
	})
	got := a.getDocument(token, doc.Id)
	require.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet())
	return got
}

// The owner's case: a CV read before is read again (Review details does it
// when its reading is out of date); its text extraction fails for good. It
// goes back to the details read before, saying the new reading failed, and
// reading it again works, never with two readings at once.
func TestAReadingAgainThatFailsForGoodKeepsTheDetailsReadBefore(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	doc := readParsedCV(t, a, conn, token, "Siti")

	res := a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	res = a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "asked again while it is read: answered, %s", res.Body)
	assert.Equal(t, 1, activeReadingJobs(t, doc.Id), "never a second reading at once")

	failAIJob(t, conn, claimAIJobFor(t, conn, doc.Id), false, "DownloadError: download failed with HTTP 500")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet(), "not \"being read\" forever")
	assert.Equal(t, keptWarning, got.ParseError.MustGet())
	assert.Equal(t, "Siti Aisyah", got.ParsedProfile.MustGet().FullName.MustGet().Value, "the details read before are kept")

	// Read again, and this time the parser reads nothing: the details read before stay too.
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil).Status)
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc.Id, Profile: nullable.NewNullNullable[api.ParsedProfile](),
		ErrorMessage: nullable.NewNullableWithValue("The details could not be read from this CV (IndexError in sections.py:41)."),
	})
	got = a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet())
	assert.Equal(t, "The CV could not be read again, so the details read before are kept: The details could not be read from this CV (IndexError in sections.py:41).", got.ParseError.MustGet())
	assert.Equal(t, "Siti Aisyah", got.ParsedProfile.MustGet().FullName.MustGet().Value, "a failed reading again never wipes them")
}

// A new CV whose file is gone: the worker gives up at once (404), and the
// CV says so instead of reading forever. Reading it again is offered, signs
// a new link, and is queued once however often it is asked for.
func TestACVWhoseFileIsGoneFailsAtOnceAndCanBeReadAgain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Nurul"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)

	failAIJob(t, conn, claimAIJobFor(t, conn, doc.Id), true, "FileMissingError: file missing (HTTP 404): it is not on the server any more; upload it again")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentStatusFailed, got.Status)
	assert.Equal(t, fileMissing, got.ErrorMessage.MustGet())

	res = a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var again api.Document
	res.JSON(&again)
	assert.Equal(t, api.DocumentStatusQueued, again.Status, "its text is read from the file again")
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPost, "/v1/documents/"+doc.Id+"/parse", token, nil).Status,
		"asked again before its text is read: nothing to read yet, as for a new upload")
	assert.Equal(t, 1, activeReadingJobs(t, doc.Id), "never a second reading")
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	assert.Equal(t, api.DocumentStatusExtracted, a.getDocument(token, doc.Id).Status)
	_, err = testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
}

// Reading the details fails for good with none read before: failed, saying so.
// A Word CV's PDF copy that fails for good: failed (the Word file is fine).
func TestDetailsAndPDFCopiesThatFailForGoodEnd(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Lim"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, parsedCV))
	failAIJob(t, conn, claimAIJobFor(t, conn, doc.Id), false, "RuntimeError: killed")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentParseStatusFailed, got.ParseStatus.MustGet())
	assert.Equal(t, noDetails, got.ParseError.MustGet())

	res = a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.docx", minimalDOCX(t))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var word api.Document
	res.JSON(&word)
	// Ended by anything (an operator here): the database tells the Go worker.
	tag, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'dead', finished_at = now()
		WHERE kind = 'document.render_pdf' AND payload->>'documentId' = $1`, word.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.PdfStatusFailed, a.getDocument(token, word.Id).PdfStatus.MustGet())
	_, err = testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
}

// Documents left "being read" with no job to finish them (from before this
// fix, a crash, a restore) are set right by the worker's upkeep: back to the
// details read before, or failed with reading again offered. One still
// being read is left alone.
func TestTheUpkeepEndsReadingsWithNoJobLeft(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	parsed := readParsedCV(t, a, conn, token, "Siti")
	c := a.createCandidate(token, map[string]any{"firstName": "Lim"})
	var fresh, waiting api.Document
	for i, d := range []*api.Document{&fresh, &waiting} {
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", pdfWith(c.Id+[]string{"a", "b"}[i]))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		res.JSON(d)
	}
	// The owner's database: a reading queued long ago, its job gone.
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE document SET parse_status = 'queued', parse_error = NULL, updated_at = now() - interval '1 hour'
		WHERE id = $1`, parsed.Id)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE payload->>'documentId' = $1`, fresh.Id)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE document SET updated_at = now() - interval '1 hour' WHERE id = ANY ($1)`, []string{fresh.Id, waiting.Id})
	require.NoError(t, err)

	n, err := a.modules.Documents.ReconcileReading(ctx, 500)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 2)
	got := a.getDocument(token, parsed.Id)
	assert.Equal(t, api.DocumentParseStatusParsed, got.ParseStatus.MustGet())
	assert.Equal(t, keptWarning, got.ParseError.MustGet())
	assert.Equal(t, "Siti Aisyah", got.ParsedProfile.MustGet().FullName.MustGet().Value)
	got = a.getDocument(token, fresh.Id)
	assert.Equal(t, api.DocumentStatusFailed, got.Status)
	assert.Equal(t, readingLost, got.ErrorMessage.MustGet())
	assert.Equal(t, api.DocumentStatusQueued, a.getDocument(token, waiting.Id).Status, "its job is still waiting: left alone")
	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'document.reading_reconciled'`, tenant.ID).Scan(&audited))
	assert.Equal(t, 2, audited)
	_, err = testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
}

// Review of 1.47.0 (before it): a CV whose reading kills the AI worker (a
// crash, out of memory) was claimed again whenever its lease ran out, for
// ever, and stayed "Reading details..." The claim after its last attempt
// ends it, and the CV says its text could not be read.
func TestACVWhoseReadingKeepsKillingTheWorkerEnds(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	c := a.createCandidate(token, map[string]any{"firstName": "Hafiz"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", pdfWith(c.Id))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	job := claimAIJobFor(t, conn, doc.Id)
	// Its worker died on its last attempt: nothing recorded, the lease ran out.
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET max_attempts = attempts, locked_until = now() - interval '1 second' WHERE id = $1`, job.ID)
	require.NoError(t, err)
	var claimed int
	require.NoError(t, testutil.Current.App.QueryRow(ctx, `SELECT count(*) FROM job_claim_for_tenant('ai', 'python-test', 60, $1)`, tenant.ID).Scan(&claimed))
	assert.Zero(t, claimed, "not claimed again")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.getDocument(token, doc.Id)
	assert.Equal(t, api.DocumentStatusFailed, got.Status, "not being read for ever")
	assert.Contains(t, got.ErrorMessage.MustGet(), "could not be read")
}

// Review of 1.47.0: a refused link (expired, or signed by another server)
// told people the file was gone and to upload it again, though the file
// was there; and a 404 from something else than the file server too. A
// dead link says to read it again; the other is a setting, tried again.
func TestADeadLinkSaysReadItAgainNotUploadItAgain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	})
	c := a.createCandidate(token, map[string]any{"firstName": "Ravi"})
	for reason, want := range map[string]string{
		"LinkExpiredError: link expired (HTTP 403): the file's link no longer works; read it again":                                  "The file's link expired before the reader could fetch it (the file itself is fine). Read it again.",
		"DownloadError: download failed: answer 404 from something other than HyPlug's file server (check HP_INTERNAL_BASE_URL)": "The CV's text could not be read: the reader stopped after several tries. Read it again; if it fails again, upload the file again.",
	} {
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", pdfWith(reason))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var doc api.Document
		res.JSON(&doc)
		failAIJob(t, conn, claimAIJobFor(t, conn, doc.Id), strings.HasPrefix(reason, "LinkExpired"), reason)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		got := a.getDocument(token, doc.Id)
		assert.Equal(t, api.DocumentStatusFailed, got.Status)
		assert.Equal(t, want, got.ErrorMessage.MustGet(), reason)
	}
}
