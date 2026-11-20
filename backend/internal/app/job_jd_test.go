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

	t.Run("only the files a JD comes in; only for those who edit jobs; only your company's", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, a.upload("/v1/jobs/"+job.Id+"/jd-files", token, "", "slides.pptx", []byte("PK\x03\x04 not a JD")).Status)
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

func TestTheAIReadsAJobDescriptionsDetailsAndCountsItsTokens(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	ctx := context.Background()
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	res := a.upload("/v1/jd-files", token, "", "JD.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var f api.JdFile
	res.JSON(&f)
	claimed := claimAIJobWhere(t, aiConn, "jdFileId", f.Id)
	var asked api.JobReadJdJob
	require.NoError(t, json.Unmarshal(claimed.Payload, &asked))
	require.NotNil(t, asked.UseAi)
	assert.True(t, *asked.UseAi, "the AI reads the details too when it is on")

	letterhead := "MAJU FOODS BERHAD\nDriving Growth Across Asia\nWe are hiring 2 planners for our Shah Alam plant.\n" +
		"Senior Production Planner\nPay: RM 7,000 - 8,500 a month"
	tokens := 900
	answerAIJob(t, a, aiConn, claimed, api.JobKindJobJdRead, api.JobJdReadJob{
		JdFileId: f.Id, Status: api.ExtractionStatusExtracted, Text: nullable.NewNullableWithValue(letterhead),
		ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens,
		AiDetails: nullable.NewNullableWithValue(api.JdAiDetails{
			Title: nullable.NewNullableWithValue("Senior Production Planner"), CompanyName: nullable.NewNullableWithValue("Maju Foods Berhad"),
			Location: nullable.NewNullableWithValue("Shah Alam"), Country: nullable.NewNullableWithValue("MY"),
			WorkType:        nullable.NewNullableWithValue(api.WorkTypeFullTime),
			WorkArrangement: nullable.NewNullableWithValue(api.WorkArrangementOnsite), Openings: nullable.NewNullableWithValue(2),
		}),
	})
	var read api.JdFile
	a.request(http.MethodGet, "/v1/jd-files/"+f.Id, token, nil).JSON(&read)
	d := read.Details.MustGet()
	assert.Equal(t, "Senior Production Planner", d.Title.MustGet(), "the AI's title, not the letterhead or the slogan")
	assert.Equal(t, "Maju Foods Berhad", d.CompanyName.MustGet())
	assert.Equal(t, 2, d.Openings.MustGet())
	assert.Equal(t, api.WorkTypeFullTime, d.WorkType.MustGet())
	assert.Equal(t, "RM 7,000 - 8,500 a month", d.SalaryText.MustGet(), "pay from the rules")

	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 900, status.UsedTokens, "reading job descriptions is in the monthly allowance")
}
