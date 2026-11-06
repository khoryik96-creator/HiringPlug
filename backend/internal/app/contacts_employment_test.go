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

func parsedJob(title, company, start string, current bool) api.ParsedJob {
	j := api.ParsedJob{Title: sourced(title, title), Company: sourced(company, company), Dates: nullable.NewNullNullable[api.SourcedText](),
		StartDate: nullable.NewNullableWithValue(start), EndDate: nullable.NewNullNullable[string](), Current: current}
	if !current {
		j.EndDate = nullable.NewNullableWithValue("2026-03")
	}
	return j
}

func linkedinProfile(name, url string, jobs ...api.ParsedJob) api.ParsedProfile {
	none := nullable.NewNullNullable[api.SourcedText]()
	return api.ParsedProfile{
		Parser: "rules@1", FullName: sourced(name, name), Email: none, Phone: none, LinkedinUrl: sourced(url, url), Location: none,
		Nationality: none, ResidenceCountry: none, Industry: none, NoticePeriod: none,
		Skills: []api.SourcedText{}, Experience: jobs, Education: []api.ParsedEducation{},
	}
}

func (a *testApp) contact(token, id string) api.Contact {
	a.t.Helper()
	var c api.Contact
	res := a.request(http.MethodGet, "/v1/contacts/"+id, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&c)
	return c
}

func TestAContactsLinkedInProfileShowsWhereTheyWorkNow(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	company := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd"})
	ct := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Nur", "lastName": "Aina", "title": "HR Manager"})
	assert.True(t, ct.Employment.IsNull(), "no check yet")

	res := a.upload("/v1/contacts/"+ct.Id+"/linkedin-profile", token, "", "notes.txt", []byte("not a pdf"))
	assert.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusForbidden, a.upload("/v1/contacts/"+ct.Id+"/linkedin-profile", tenant.Users[api.RoleViewer].Token, "", "Profile.pdf", minimalPDF).Status)

	res = a.upload("/v1/contacts/"+ct.Id+"/linkedin-profile", token, "", "Profile.pdf", minimalPDF)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var check api.ContactEmployment
	res.JSON(&check)
	assert.Equal(t, api.ContactEmploymentStatusReading, check.Status)
	assert.Equal(t, api.ContactEmploymentVerdictUnknown, check.Verdict)
	assert.Equal(t, check.Id, a.contact(token, ct.Id).Employment.MustGet().Id)

	job := claimAIJobWhere(t, aiConn, "checkId", check.Id)
	require.Equal(t, string(api.JobKindContactReadLinkedin), job.Kind)
	var read api.ContactReadLinkedinJob
	require.NoError(t, json.Unmarshal(job.Payload, &read))
	assert.Equal(t, "Profile.pdf", read.FileName)
	assert.NotEmpty(t, read.DownloadUrl)

	answerAIJob(t, a, aiConn, job, api.JobKindContactLinkedinRead, api.ContactLinkedinReadJob{
		CheckId: check.Id, ErrorMessage: nullable.NewNullNullable[string](),
		Profile: nullable.NewNullableWithValue(linkedinProfile("Nur Aina binti Ahmad", "linkedin.com/in/nur-aina-example",
			parsedJob("Talent Acquisition Lead", "Contoh Bank Berhad", "2026-04", true),
			parsedJob("HR Manager", "MAJU SDN. BHD.", "2021-03", false))),
	})

	got := a.contact(token, ct.Id)
	e := got.Employment.MustGet()
	assert.Equal(t, api.ContactEmploymentStatusRead, e.Status)
	assert.Equal(t, api.ContactEmploymentVerdictMoved, e.Verdict, "their current job is at another company")
	assert.Equal(t, "Contoh Bank Berhad", e.Current.MustGet().Company.MustGet())
	assert.Len(t, e.Positions, 2)
	assert.True(t, e.NameMatches.MustGet())
	assert.Equal(t, "https://www.linkedin.com/in/nur-aina-example", got.LinkedinUrl.MustGet(), "the missing LinkedIn address is filled in")
	assert.True(t, e.UrlMatches.MustGet())
	assert.Equal(t, ct.Version+1, got.Version)

	var key *string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT storage_key FROM contact_employment_check WHERE id = $1`, check.Id).Scan(&key))
	assert.Nil(t, key, "the PDF is deleted once read")

	t.Run("the comparison follows the contact", func(t *testing.T) {
		bank := a.createCompany(token, map[string]any{"name": "Contoh Bank"})
		res := a.request(http.MethodPatch, "/v1/contacts/"+ct.Id, token, map[string]any{"version": got.Version, "companyId": bank.Id})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, api.ContactEmploymentVerdictSameCompany, a.contact(token, ct.Id).Employment.MustGet().Verdict)
	})

	t.Run("an unreadable file says so", func(t *testing.T) {
		res := a.upload("/v1/contacts/"+ct.Id+"/linkedin-profile", token, "", "scan.pdf", minimalPDF)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var c2 api.ContactEmployment
		res.JSON(&c2)
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "checkId", c2.Id), api.JobKindContactLinkedinRead, api.ContactLinkedinReadJob{
			CheckId: c2.Id, Profile: nullable.NewNullNullable[api.ParsedProfile](), ErrorMessage: nullable.NewNullableWithValue("no text was found in it"),
		})
		e := a.contact(token, ct.Id).Employment.MustGet()
		assert.Equal(t, api.ContactEmploymentStatusFailed, e.Status)
		assert.Equal(t, "It could not be read: no text was found in it", e.Error.MustGet())
	})

	t.Run("a check never read is given up after an hour, so the PDF can be dropped again", func(t *testing.T) {
		other := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Lim"})
		res := a.upload("/v1/contacts/"+other.Id+"/linkedin-profile", token, "", "Profile.pdf", minimalPDF)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var c3 api.ContactEmployment
		res.JSON(&c3)
		assert.Equal(t, api.ContactEmploymentStatusReading, a.contact(token, other.Id).Employment.MustGet().Status)
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE contact_employment_check SET created_at = now() - interval '2 hours' WHERE id = $1`, c3.Id)
		require.NoError(t, err)
		e := a.contact(token, other.Id).Employment.MustGet()
		assert.Equal(t, api.ContactEmploymentStatusFailed, e.Status)
		assert.Equal(t, "It was not read in time: drop the PDF again.", e.Error.MustGet())
		claimAIJobWhere(t, aiConn, "checkId", c3.Id) // leave the queue tidy for other tests
	})

	t.Run("another company cannot check this contact", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.upload("/v1/contacts/"+ct.Id+"/linkedin-profile", other, "", "Profile.pdf", minimalPDF).Status)
	})
}
