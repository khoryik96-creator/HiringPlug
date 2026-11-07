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

const financeJD = `Position: Finance Manager
Location: Petaling Jaya, Selangor
Salary: RM 12,000 - 15,000 per month
Employment Type: Permanent
Work arrangement: Hybrid

Maju Foods Berhad is looking for a Finance Manager.

Responsibilities
• Lead month-end closing`

// The client's job description is dropped on the ad form: its text is read
// (a text file at once, a PDF by the AI worker) and the ad's details come back.
func TestJobDescriptionFillsTheAd(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance"})
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token

	t.Run("a text file is read at once", func(t *testing.T) {
		res := a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "Finance Manager JD.txt", []byte(financeJD))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var f api.JdFile
		res.JSON(&f)
		assert.Equal(t, api.JdFileStatusRead, f.Status)
		d := f.Details.MustGet()
		assert.Equal(t, "Finance Manager", d.Title.MustGet())
		assert.Equal(t, "Petaling Jaya, Selangor", d.Location.MustGet())
		assert.Equal(t, "MY", d.Country.MustGet())
		assert.Equal(t, "RM 12,000 - 15,000 per month", d.SalaryText.MustGet())
		assert.Equal(t, api.WorkTypeFullTime, d.WorkType.MustGet())
		assert.Equal(t, api.WorkArrangementHybrid, d.WorkArrangement.MustGet())
		assert.True(t, d.ClosesOn.IsNull())
		assert.Contains(t, d.Description, "Responsibilities\n- Lead month-end closing")
	})

	t.Run("a PDF is read by the AI worker, then its details are read", func(t *testing.T) {
		res := a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "JD.pdf", minimalPDF)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var f api.JdFile
		res.JSON(&f)
		assert.Equal(t, api.JdFileStatusReading, f.Status)
		assert.True(t, f.Details.IsNull())

		ctx := context.Background()
		aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
		require.NoError(t, err)
		defer func() { _ = aiConn.Close(ctx) }()
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "jdFileId", f.Id), api.JobKindJobJdRead, api.JobJdReadJob{
			JdFileId: f.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue(financeJD),
			ErrorMessage: nullable.NewNullNullable[string](),
		})
		var read api.JdFile
		a.request(http.MethodGet, "/v1/jd-files/"+f.Id, tenant.Users[api.RoleViewer].Token, nil).JSON(&read)
		assert.Equal(t, api.JdFileStatusRead, read.Status)
		assert.Equal(t, "Finance Manager", read.Details.MustGet().Title.MustGet())
	})

	t.Run("a scan with no text says so", func(t *testing.T) {
		res := a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "scan.pdf", minimalPDF)
		var f api.JdFile
		res.JSON(&f)
		ctx := context.Background()
		aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
		require.NoError(t, err)
		defer func() { _ = aiConn.Close(ctx) }()
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "jdFileId", f.Id), api.JobKindJobJdRead, api.JobJdReadJob{
			JdFileId: f.Id, Status: api.ExtractionStatusNeedsOcr, Text: nullable.NewNullNullable[string](), ErrorMessage: nullable.NewNullNullable[string](),
		})
		var read api.JdFile
		a.request(http.MethodGet, "/v1/jd-files/"+f.Id, token, nil).JSON(&read)
		assert.Equal(t, api.JdFileStatusFailed, read.Status)
		assert.Contains(t, read.Error.MustGet(), "scan with no text")
	})

	t.Run("only PDF, Word and text files; only for those who edit jobs; only your company's", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "photo.png", bannerPNG).Status)
		assert.Equal(t, http.StatusBadRequest, a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "fake.pdf", []byte("hello")).Status)
		assert.Equal(t, http.StatusForbidden, a.upload("/v1/jobs/"+job.Id+"/jd-files", tenant.Users[api.RoleViewer].Token, "", "jd.txt", []byte(financeJD)).Status)
		assert.Equal(t, http.StatusNotFound, a.upload("/v1/jobs/"+job.Id+"/jd-files", other, "", "jd.txt", []byte(financeJD)).Status)
	})
}

// Dropped on the Add job form: read before the job exists, to fill it in.
func TestAJobDescriptionFillsInANewJob(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token

	res := a.upload("/v1/jd-files", token, "", "Finance Manager JD.txt", []byte(financeJD))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var f api.JdFile
	res.JSON(&f)
	assert.True(t, f.JobId.IsNull(), "no job yet")
	assert.Equal(t, api.JdFileStatusRead, f.Status)
	assert.Equal(t, "Finance Manager", f.Details.MustGet().Title.MustGet())

	res = a.upload("/v1/jd-files", token, "", "JD.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&f)
	ctx := context.Background()
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "jdFileId", f.Id), api.JobKindJobJdRead, api.JobJdReadJob{
		JdFileId: f.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue(financeJD),
		ErrorMessage: nullable.NewNullNullable[string](),
	})
	var read api.JdFile
	a.request(http.MethodGet, "/v1/jd-files/"+f.Id, token, nil).JSON(&read)
	assert.Equal(t, api.JdFileStatusRead, read.Status)
	assert.Equal(t, api.WorkArrangementHybrid, read.Details.MustGet().WorkArrangement.MustGet())

	assert.Equal(t, http.StatusForbidden, a.upload("/v1/jd-files", tenant.Users[api.RoleViewer].Token, "", "jd.txt", []byte(financeJD)).Status,
		"only those who may add jobs")
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jd-files/"+f.Id, other, nil).Status, "only your company's")
}
