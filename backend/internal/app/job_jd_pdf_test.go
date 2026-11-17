package app_test

import (
	"context"
	"encoding/base64"
	"net/http"
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
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
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

	t.Run("another company's file is not found", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+f.Id+"/pdf", other, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/jd-files/"+f.Id+"/pdf", other, nil).Status)
	})
}
