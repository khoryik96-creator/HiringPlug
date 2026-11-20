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

// readCV uploads a CV, answers its text extraction and returns the parse job.
func readCV(t *testing.T, a *testApp, conn *pgx.Conn, token, text string) (api.Document, claimedJob, api.DocumentParseCvJob) {
	t.Helper()
	c := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, conn, claimAIJobFor(t, conn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, text))
	job := claimAIJobFor(t, conn, doc.Id)
	require.Equal(t, "document.parse_cv", job.Kind)
	var payload api.DocumentParseCvJob
	require.NoError(t, json.Unmarshal(job.Payload, &payload))
	return doc, job, payload
}

func TestTheAIFillsCVReadingsOnlyWhenOnAndCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	off := newTestApp(t)
	offToken := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	_, _, payload := readCV(t, off, conn, offToken, "SITI AISYAH")
	assert.False(t, *payload.UseAi, "the AI is off")
	assert.Nil(t, payload.AiText)

	on := newTestApp(t, assistantOn)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	doc, job, payload := readCV(t, on, conn, token, "SITI AISYAH works at Contoh Sdn Bhd\nExpected salary: RM 8,500")
	assert.True(t, *payload.UseAi)
	require.NotNil(t, payload.AiText)
	assert.Equal(t, "SITI AISYAH works at Contoh Sdn Bhd\nExpected [salary removed]", *payload.AiText, "salary never goes to the AI")
	assert.Contains(t, payload.Text, "8,500", "quotes still come from the saved text")

	tokens := 1700
	answerAIJob(t, on, conn, job, api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](), AiTokens: &tokens,
		Profile: nullable.NewNullableWithValue(api.ParsedProfile{
			Parser: "rules@1+ai", FullName: sourced("Siti Aisyah", "SITI AISYAH"), Industry: sourced("Professional Services", "Contoh Sdn Bhd"),
			Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
		}),
	})
	got := on.getDocument(token, doc.Id)
	assert.True(t, got.ParsedProfile.MustGet().Industry.IsNull(), "unverified provider sectors cannot classify an employer without employment evidence")

	var status api.AssistantStatus
	on.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 1700, status.UsedTokens, "one monthly allowance covers reading CVs too")

	t.Run("no AI once the allowance is used", func(t *testing.T) {
		_, _, payload := readCV(t, on, conn, token, "AHMAD ALI") // the allowance is 5000; add another 4000
		require.True(t, *payload.UseAi)
		_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO document_ai_use (tenant_id, document_id, tokens) VALUES ($1, $2, 4000)`, tenant.ID, doc.Id)
		require.NoError(t, err)
		_, _, payload = readCV(t, on, conn, token, "BALA KUMAR")
		assert.False(t, *payload.UseAi)
	})
}

func TestTheAIsPickOfTheNewestEmployersIndustryIsTaggedWhenTheNameRulesCannotTell(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	a := newTestApp(t, assistantOn)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	text := "SITI AISYAH\nSenior Analyst | Kenanga Contoh Holdings | Jan 2021 - Present\nAnalyst | Syarikat Lama | 2018 - 2020"
	job := func(title, company, dates, start, end string, current bool) api.ParsedJob {
		j := api.ParsedJob{Title: sourced(title, title), Company: sourced(company, company), Dates: sourced(dates, dates),
			StartDate: nullable.NewNullableWithValue(start), EndDate: nullable.NewNullNullable[string](), Current: current}
		if end != "" {
			j.EndDate = nullable.NewNullableWithValue(end)
		}
		return j
	}
	answer := func(industry nullable.Nullable[api.SourcedText]) api.ParsedProfile {
		return api.ParsedProfile{Parser: "rules@1+ai-history", FullName: sourced("Siti Aisyah", "SITI AISYAH"), Industry: industry,
			Skills: []api.SourcedText{}, Education: []api.ParsedEducation{}, Experience: []api.ParsedJob{
				job("Senior Analyst", "Kenanga Contoh Holdings", "Jan 2021 - Present", "2021-01", "", true),
				job("Analyst", "Syarikat Lama", "2018 - 2020", "2018", "2020", false),
			}}
	}
	parse := func(profile api.ParsedProfile) api.ParsedProfile {
		doc, claimed, payload := readCV(t, a, conn, token, text)
		require.NotNil(t, payload.Industries, "the company's industry list goes with the AI read")
		assert.Contains(t, *payload.Industries, "FSI – Banking")
		answerAIJob(t, a, conn, claimed, api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
			DocumentId: doc.Id, ErrorMessage: nullable.NewNullNullable[string](), Profile: nullable.NewNullableWithValue(profile)})
		return a.getDocument(token, doc.Id).ParsedProfile.MustGet()
	}

	got := parse(answer(sourced("FSI – Banking", "Kenanga Contoh Holdings")))
	require.True(t, got.Industry.IsSpecified() && !got.Industry.IsNull(), "tagged from the AI's pick")
	assert.Equal(t, "Financial Services", got.Industry.MustGet().Value, "the industry the pick belongs to")
	assert.Equal(t, 0, *got.SelectedEmployerIndex)

	got = parse(answer(sourced("FSI – Banking", "Syarikat Lama")))
	assert.True(t, got.Industry.IsNull(), "a pick for another employer than the newest is not used")
	got = parse(answer(sourced("Space Mining", "Kenanga Contoh Holdings")))
	assert.True(t, got.Industry.IsNull(), "a pick that is not on the list is not used")
}
