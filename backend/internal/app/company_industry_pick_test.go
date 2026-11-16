package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A new company without an industry gets the closest of the agency's
// industries, as the AI picks it from what is known of the company (and an
// industry named on LinkedIn as a hint); one with an industry keeps it.
func TestTheAIPicksANewCompanysIndustry(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, assistantOn)
	conn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	name := fmt.Sprintf("Maju Logistik %d", time.Now().UnixNano())
	c := a.createCompany(token, map[string]any{"name": name, "industryHint": "Truck Transportation",
		"about": "We move goods across the peninsula with 300 trucks."})
	assert.True(t, *c.IndustryPending, "the pick is under way: the page reads the company again")
	var n int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		`SELECT count(*) FROM company_industry_pick WHERE company_id = $1`, c.Id).Scan(&n))
	assert.Zero(t, n, "adding the company only queues the ask (no AI bookkeeping while it is added)")
	runCoreJobsUntilIdle(t, a, tenant.ID)

	job := claimAIJobWhere(t, conn, "companyName", name)
	require.Equal(t, string(api.JobKindCompanyIndustry), job.Kind)
	var asked api.CompanyIndustryJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	assert.Equal(t, "Truck Transportation", asked.Hint.MustGet(), "LinkedIn's industry, as a hint")
	assert.Contains(t, asked.About.MustGet(), "300 trucks")
	require.NotEmpty(t, asked.Choices)
	var pick api.CompanyIndustryChoice
	for _, ch := range asked.Choices {
		if strings.Contains(ch.Label, " > ") {
			pick = ch // a subindustry
			break
		}
	}
	require.NotEmpty(t, pick.Id, "the choices include subindustries")

	answerAIJob(t, a, conn, job, api.JobKindCompanyIndustried, api.CompanyIndustriedJob{
		PickId: asked.PickId, ChoiceId: nullable.NewNullableWithValue(pick.Id), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("deepseek-chat"), InputTokens: 300, OutputTokens: 2,
	})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var got api.Company
	a.request(http.MethodGet, "/v1/companies/"+c.Id, token, nil).JSON(&got)
	require.NotNil(t, got.IndustrySelections)
	assert.Equal(t, []string{pick.Id}, got.IndustrySelections.Ids)
	assert.Equal(t, strings.SplitN(pick.Label, " > ", 2)[0], got.Industry.MustGet(), "shown as its industry")
	assert.False(t, *got.IndustryPending, "picked: no longer under way")
	var catalogue api.IndustryCatalogue
	a.request(http.MethodGet, "/v1/industries", token, nil).JSON(&catalogue)
	for _, i := range catalogue.Items {
		assert.NotEqual(t, "Truck Transportation", i.Label, "LinkedIn's industry is a hint, never a new industry")
	}
	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 302, status.UsedTokens, "it counts towards the month's allowance")

	t.Run("with a website, it is asked once the website is read, with what the website says", func(t *testing.T) {
		site := companySite(t, false)
		named := fmt.Sprintf("Contoh Payroll %d", time.Now().UnixNano())
		a.createCompany(token, map[string]any{"name": named, "website": site.URL, "fillFromWebsite": true})
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
			`SELECT count(*) FROM job WHERE queue = 'ai' AND payload->>'companyName' = $1`, named).Scan(&n))
		assert.Zero(t, n, "not before the website is read")
		runCoreJobsUntilIdle(t, a, tenant.ID)
		job := claimAIJobWhere(t, conn, "companyName", named)
		var asked api.CompanyIndustryJob
		require.NoError(t, json.Unmarshal(job.Payload, &asked))
		assert.Equal(t, "Payroll software for Malaysian companies.", asked.About.MustGet(), "what the website says of the company")
	})

	t.Run("a website that cannot be read still leaves the industry to the AI", func(t *testing.T) {
		closed := companySite(t, true)
		named := fmt.Sprintf("Closed Co %d", time.Now().UnixNano())
		a.createCompany(token, map[string]any{"name": named, "website": closed.URL, "fillFromWebsite": true})
		runCoreJobsUntilIdle(t, a, tenant.ID)
		job := claimAIJobWhere(t, conn, "companyName", named)
		assert.Equal(t, string(api.JobKindCompanyIndustry), job.Kind)
	})

	t.Run("a company whose industry was chosen keeps it", func(t *testing.T) {
		chosen := fmt.Sprintf("Chosen Co %d", time.Now().UnixNano())
		kept := a.createCompany(token, map[string]any{"name": chosen,
			"industrySelections": map[string]any{"catalogueVersion": got.IndustrySelections.CatalogueVersion, "ids": []string{pick.Id}, "primaryId": pick.Id}})
		assert.False(t, *kept.IndustryPending)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
			`SELECT count(*) FROM job WHERE queue = 'ai' AND payload->>'companyName' = $1`, chosen).Scan(&n))
		assert.Zero(t, n, "not asked")
	})
}
