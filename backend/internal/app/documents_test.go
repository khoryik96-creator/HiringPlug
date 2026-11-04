package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var claimLock sync.Mutex

var minimalPDF = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")

func minimalDOCX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create("word/document.xml")
	require.NoError(t, err)
	_, err = f.Write([]byte(`<w:document><w:body><w:p><w:r><w:t>CV</w:t></w:r></w:p></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, z.Close())
	return buf.Bytes()
}

// TestCVUploadToExtractedText follows a CV through the whole pipeline:
// upload -> job for the Python worker -> its result job -> Go worker -> text
// on the document. The Python side is played by SQL run as the AI role, with
// exactly the calls the real worker makes.
func TestCVUploadToExtractedText(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Nurul"})
	docsPath := "/v1/candidates/" + c.Id + "/documents"

	res := a.upload(docsPath, token, "jobstreet", "Nurul CV.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	assert.Equal(t, api.DocumentStatusQueued, doc.Status)
	assert.Equal(t, "application/pdf", doc.ContentType)
	assert.Equal(t, "Nurul CV.pdf", doc.FileName)
	requestID := res.Header.Get("X-Request-Id")

	// The Python worker's view: claim the job with the AI role.
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	job := claimAIJobFor(t, aiConn, doc.Id)
	assert.Equal(t, "document.extract_text", job.Kind)
	assert.Equal(t, requestID, job.RequestID, "the job carries the upload's request ID")

	var payload api.DocumentExtractTextJob
	require.NoError(t, json.Unmarshal(job.Payload, &payload))
	assert.Equal(t, doc.Id, payload.DocumentId)

	t.Run("the signed link downloads the file, and only with a valid signature", func(t *testing.T) {
		link, err := url.Parse(payload.DownloadUrl)
		require.NoError(t, err)
		got := a.request(http.MethodGet, link.RequestURI(), "", nil)
		require.Equal(t, http.StatusOK, got.Status)
		assert.Equal(t, minimalPDF, got.Body)

		tampered := strings.Replace(link.RequestURI(), "sig=", "sig=0", 1)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, tampered, "", nil).Status)
	})

	t.Run("the AI role can use the queue but cannot read candidate data", func(t *testing.T) {
		_, err := aiConn.Exec(ctx, "SELECT count(*) FROM candidate")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("text with NUL characters breaks the contract (the worker must remove them)", func(t *testing.T) {
		bad := api.DocumentTextExtractedJob{
			DocumentId: doc.Id, Status: api.ExtractionStatusExtracted, Extractor: "test@1",
			Text: nullable.NewNullableWithValue("a\x00b"), PageCount: nullable.NewNullNullable[int](),
			ErrorMessage: nullable.NewNullNullable[string](), Warnings: []string{},
		}
		err := database.InTx(ctx, a.deps.Pool, authz.System(job.TenantID), func(tx pgx.Tx) error {
			_, err := jobqueue.Enqueue(ctx, tx, job.TenantID, api.JobKindDocumentTextExtracted, bad)
			return err
		})
		require.ErrorContains(t, err, "does not match schema")
	})

	// The Python worker answers with a result job and completes its own job,
	// in one transaction.
	result, err := json.Marshal(api.DocumentTextExtractedJob{
		DocumentId: doc.Id, Status: api.ExtractionStatusExtracted,
		Text:         nullable.NewNullableWithValue("Nurul  Senior Accountant"),
		PageCount:    nullable.NewNullableWithValue(2),
		Extractor:    "test@1",
		ErrorMessage: nullable.NewNullNullable[string](),
		Warnings:     []string{"OCR could not read page 2"},
	})
	require.NoError(t, err)
	tx, err := aiConn.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT job_enqueue($1, 'core', 'document.text_extracted', $2::jsonb, $3)`, job.TenantID, string(result), job.RequestID)
	require.NoError(t, err)
	var completed bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT job_complete($1, 'python-test')`, job.ID).Scan(&completed))
	require.True(t, completed)
	require.NoError(t, tx.Commit(ctx))

	// The Go worker saves the result.
	runCoreJobsUntilIdle(t, a, job.TenantID)

	got := a.request(http.MethodGet, "/v1/documents/"+doc.Id, token, nil)
	require.Equal(t, http.StatusOK, got.Status)
	var extracted api.Document
	got.JSON(&extracted)
	assert.Equal(t, api.DocumentStatusExtracted, extracted.Status)
	assert.Equal(t, "Nurul  Senior Accountant", extracted.ExtractedText.MustGet())
	assert.Equal(t, 2, extracted.PageCount.MustGet())
	assert.Equal(t, []string{"OCR could not read page 2"}, extracted.ExtractionWarnings)

	list := a.request(http.MethodGet, docsPath, token, nil).Map()
	items := list["items"].([]any)
	require.Len(t, items, 1)
	assert.NotContains(t, items[0], "extractedText", "lists leave out the text")
}

func TestUploadRejectsBadFiles(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Lim"})
	path := "/v1/candidates/" + c.Id + "/documents"

	t.Run("a Word document is accepted", func(t *testing.T) {
		res := a.upload(path, token, "original", "cv.docx", minimalDOCX(t))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	})
	t.Run("a text file pretending to be a PDF is refused", func(t *testing.T) {
		res := a.upload(path, token, "original", "cv.pdf", []byte("just text"))
		require.Equal(t, http.StatusUnsupportedMediaType, res.Status)
	})
	t.Run("a file over the limit is refused", func(t *testing.T) {
		big := append(append([]byte{}, minimalPDF...), bytes.Repeat([]byte("x"), testMaxUpload)...)
		res := a.upload(path, token, "original", "big.pdf", big)
		require.Equal(t, http.StatusRequestEntityTooLarge, res.Status)
	})
	t.Run("source is required and must be known", func(t *testing.T) {
		assert.Equal(t, []string{"source"}, fieldNames(t, a.upload(path, token, "", "cv.pdf", minimalPDF).Error()))
		assert.Equal(t, []string{"source"}, fieldNames(t, a.upload(path, token, "monster", "cv.pdf", minimalPDF).Error()))
	})
	t.Run("an unknown candidate is a 404", func(t *testing.T) {
		res := a.upload("/v1/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7/documents", token, "original", "cv.pdf", minimalPDF)
		assert.Equal(t, http.StatusNotFound, res.Status)
	})
}

type claimedJob struct {
	ID, TenantID, Kind, RequestID string
	Payload                       []byte
}

// claimAIJobFor claims the AI-queue job for documentID exactly as the Python
// worker does (job_claim, as the AI role). Other tests may have queued AI jobs
// too, so the job is first moved to the front of the queue. Tests run in
// parallel: moving and claiming happen under one lock, or two tests could each
// move their job forward and then claim the other's.
func claimAIJobFor(t *testing.T, conn *pgx.Conn, documentID string) claimedJob {
	t.Helper()
	return claimAIJobWhere(t, conn, "documentId", documentID)
}

// claimAIJobWhere claims the AI-queue job whose payload has key = value.
func claimAIJobWhere(t *testing.T, conn *pgx.Conn, key, value string) claimedJob {
	t.Helper()
	claimLock.Lock()
	defer claimLock.Unlock()
	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx,
		`UPDATE job SET run_after = now() - interval '1 hour' WHERE queue = 'ai' AND payload->>$1 = $2`, key, value)
	require.NoError(t, err)
	var j claimedJob
	var reqID *string
	err = conn.QueryRow(ctx, `SELECT id, tenant_id, kind, payload, request_id FROM job_claim('ai', 'python-test', 60)`).
		Scan(&j.ID, &j.TenantID, &j.Kind, &j.Payload, &reqID)
	require.NoError(t, err, "no AI job found for %s", value)
	require.Contains(t, string(j.Payload), value)
	if reqID != nil {
		j.RequestID = *reqID
	}
	return j
}

// runCoreJobsUntilIdle runs the real Go worker (this test app's, with its
// settings) on the tenant's core jobs until they are done. Tests run in
// parallel and share the queue, so the worker claims only this tenant's jobs:
// another test's worker, with other settings (the AI on or off), never runs
// them.
func runCoreJobsUntilIdle(t *testing.T, a *testApp, tenantID string) {
	t.Helper()
	ctx := context.Background()
	w, err := app.NewWorker(a.deps, a.modules)
	require.NoError(t, err)
	w.Tenant = tenantID
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		found, err := w.RunOnce(ctx)
		require.NoError(t, err)
		if found {
			continue
		}
		// Jobs due now (or running); later ones, such as interview
		// reminders, wait for their time.
		var open int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE queue = 'core' AND tenant_id = $1
			AND (status = 'running' OR (status = 'pending' AND run_after <= now()))`, tenantID).Scan(&open))
		if open == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("core queue did not drain")
}

func TestDownloadingADocumentFile(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "José"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "José CV.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)

	got := a.request(http.MethodGet, "/v1/documents/"+doc.Id+"/file", tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, got.Status)
	assert.Equal(t, minimalPDF, got.Body, "the bytes as uploaded")
	assert.Equal(t, `attachment; filename*=utf-8''Jos%C3%A9%20CV.pdf`, got.Header.Get("Content-Disposition"))

	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+doc.Id+"/file", other, nil).Status,
		"another company's document does not exist for them")
	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/documents/"+doc.Id+"/file", "", nil).Status)
}
