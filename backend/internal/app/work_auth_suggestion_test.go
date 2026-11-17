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

func TestACVsNationalityAndPassSuggestAWorkAuthorizationUntilOneIsSet(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Arjun"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	text := "ARJUN NAIR\nNationality: Indian\nRP-T holder, based in Kuala Lumpur\nFinance Manager"
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, text))
	none := nullable.NewNullNullable[api.SourcedText]()
	profile := api.ParsedProfile{
		Parser: "rules@1", FullName: sourced("Arjun Nair", "ARJUN NAIR"), Email: none, Phone: none, LinkedinUrl: none, Location: none,
		Nationality: sourced("IN", "Indian"), ResidenceCountry: none, Industry: none, NoticePeriod: none,
		Skills: []api.SourcedText{}, Experience: []api.ParsedJob{}, Education: []api.ParsedEducation{},
	}
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentCvParsed, api.DocumentCvParsedJob{
		DocumentId: doc.Id, Profile: nullable.NewNullableWithValue(profile), ErrorMessage: nullable.NewNullNullable[string](),
	})

	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, token, nil).JSON(&got)
	s := got.WorkAuthorizationSuggestion.MustGet()
	assert.Equal(t, api.WorkAuthorizationStatusExpatEligible, s.Status)
	assert.Equal(t, api.WorkAuthorizationBasisRpT, s.Basis.MustGet())
	assert.Contains(t, s.Reason, "RP-T")
	assert.True(t, got.WorkAuthorization.IsNull() || !got.WorkAuthorization.IsSpecified(), "a suggestion is never set on its own")

	res = a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": got.Version,
		"workAuthorization": map[string]any{"status": "expat_eligible", "basis": "rp_t"}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&got)
	assert.True(t, got.WorkAuthorizationSuggestion.IsNull(), "once set, no suggestion")
}
