package app_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A Word job description is previewed as it looks: the AI worker makes a PDF
// copy, which Job details and the job's ads show (it showed the text read).
func TestAWordJobDescriptionGetsAPdfCopy(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance"})
	ctx := context.Background()
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	res := a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "Finance JD.docx", minimalDOCX(t))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var f api.JdFile
	res.JSON(&f)
	assert.Equal(t, api.PdfStatusQueued, f.PdfStatus.MustGet())
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+f.Id+"/pdf", token, nil).Status, "not made yet")

	pdf := []byte("%PDF-1.4 a job description")
	for range 2 { // its text is read, and its PDF made: two jobs, in either order
		j := claimAIJobWhere(t, aiConn, "jdFileId", f.Id)
		switch api.JobKind(j.Kind) {
		case api.JobKindJdRenderPdf:
			answerAIJob(t, a, aiConn, j, api.JobKindJdPdfRendered, api.JdPdfRenderedJob{
				JdFileId: f.Id, PdfBase64: nullable.NewNullableWithValue(base64.StdEncoding.EncodeToString(pdf)), Error: nullable.NewNullNullable[string](),
			})
		case api.JobKindJobReadJd:
			answerAIJob(t, a, aiConn, j, api.JobKindJobJdRead, api.JobJdReadJob{
				JdFileId: f.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue(financeJD), ErrorMessage: nullable.NewNullNullable[string](),
			})
		default:
			t.Fatalf("unexpected job %s", j.Kind)
		}
	}
	var read api.JdFile
	a.request(http.MethodGet, "/v1/jd-files/"+f.Id, tenant.Users[api.RoleViewer].Token, nil).JSON(&read)
	assert.Equal(t, api.PdfStatusReady, read.PdfStatus.MustGet())
	got := a.request(http.MethodGet, "/v1/jd-files/"+f.Id+"/pdf", tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, got.Status)
	assert.Equal(t, pdf, got.Body)

	t.Run("a PDF or a text file needs no copy", func(t *testing.T) {
		var txt api.JdFile
		a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "jd.txt", []byte(financeJD)).JSON(&txt)
		assert.True(t, txt.PdfStatus.IsNull())
		var made api.JdFile
		res := a.request(http.MethodPost, "/v1/jd-files/"+txt.Id+"/pdf", token, nil)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&made)
		assert.True(t, made.PdfStatus.IsNull(), "nothing to make")
	})

	t.Run("a Word file uploaded before previews gets one when asked", func(t *testing.T) {
		var old api.JdFile
		a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "old.docx", minimalDOCX(t)).JSON(&old)
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job_description_file SET pdf_status = NULL WHERE id = $1`, old.Id)
		require.NoError(t, err)
		var made api.JdFile
		res := a.request(http.MethodPost, "/v1/jd-files/"+old.Id+"/pdf", tenant.Users[api.RoleViewer].Token, nil)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&made)
		assert.Equal(t, api.PdfStatusQueued, made.PdfStatus.MustGet())
	})

	t.Run("two asking at once for an old file's copy queue it once", func(t *testing.T) {
		var old api.JdFile
		a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "older.docx", minimalDOCX(t)).JSON(&old)
		_, err := testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE payload->>'jdFileId' = $1 AND kind = 'jd.render_pdf'`, old.Id)
		require.NoError(t, err)
		_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job_description_file SET pdf_status = NULL WHERE id = $1`, old.Id)
		require.NoError(t, err)
		done := make(chan int, 2)
		for range 2 {
			go func() { done <- a.request(http.MethodPost, "/v1/jd-files/"+old.Id+"/pdf", token, nil).Status }()
		}
		assert.Equal(t, http.StatusAccepted, <-done)
		assert.Equal(t, http.StatusAccepted, <-done)
		var queued int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE payload->>'jdFileId' = $1 AND kind = 'jd.render_pdf'`, old.Id).Scan(&queued))
		assert.Equal(t, 1, queued, "one copy made, so a second result cannot delete the first's file")
	})

	t.Run("another company's file is not found", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+f.Id+"/pdf", other, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/jd-files/"+f.Id+"/pdf", other, nil).Status)
	})

	t.Run("a purged job takes its PDF copies with it", func(t *testing.T) {
		var key string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT pdf_key FROM job_description_file WHERE id = $1`, f.Id).Scan(&key))
		require.True(t, stored(t, key), "the copy is stored")
		res := a.bin(tenant.Users[api.RoleOwner].Token, map[string]any{"kind": "company", "id": company.Id})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		purgeNow(t, a, tenant.ID)
		assert.False(t, stored(t, key), "the PDF copy is removed with the job")
	})
}

// stored reports whether the file store holds the file under key (its tenant's folder, sharded).
func stored(t *testing.T, key string) bool {
	t.Helper()
	tenant, name, _ := strings.Cut(key, "/")
	found := false
	_ = filepath.WalkDir(filepath.Join(filesDir, tenant), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(path) == name {
			found = true
		}
		return nil
	})
	return found
}
