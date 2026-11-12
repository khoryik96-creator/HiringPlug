package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func industryCatalogue(t *testing.T, a *testApp, token string) api.IndustryCatalogue {
	t.Helper()
	r := a.request("GET", "/v1/industries", token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var c api.IndustryCatalogue
	r.JSON(&c)
	return c
}
func industryNamed(t *testing.T, c api.IndustryCatalogue, label string) api.Industry {
	t.Helper()
	for _, i := range c.Items {
		if i.Label == label {
			return i
		}
	}
	t.Fatalf("missing industry %q", label)
	return api.Industry{}
}
func TestIndustriesSeedPermissionsAndIsolation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner := tenant.Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, owner)
	require.Len(t, c.Items, 91)
	assert.Nil(t, industryNamed(t, c, "Education").ParentId)
	body := map[string]any{"catalogueVersion": c.Version, "label": "Agency niche", "aliases": []string{"niche"}, "retired": false}
	r := a.request("POST", "/v1/industries", tenant.Users[api.RoleRecruiter].Token, body)
	assert.Equal(t, 403, r.Status)
	r = a.request("POST", "/v1/industries", owner, body)
	require.Equal(t, 201, r.Status, "%s", r.Body)
	r.JSON(&c)
	niche := industryNamed(t, c, "Agency niche")
	r = a.request("PATCH", "/v1/industries/"+niche.Id, owner, body)
	assert.Equal(t, 409, r.Status, "stale catalogue cannot overwrite another change")
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	otherC := industryCatalogue(t, a, other.Users[api.RoleOwner].Token)
	require.Len(t, otherC.Items, 91)
	body["catalogueVersion"] = otherC.Version
	r = a.request("PATCH", "/v1/industries/"+niche.Id, other.Users[api.RoleOwner].Token, body)
	assert.Equal(t, 404, r.Status)
	r = a.request("POST", "/v1/industries", owner, map[string]any{"catalogueVersion": c.Version, "label": "Nested", "aliases": []string{}, "retired": false, "parentId": industryNamed(t, c, "FSI – Banking").Id})
	assert.Equal(t, 400, r.Status)
}
func TestIndustriesProfileRenameReassignAndRetirement(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleViewer)
	token := tenant.Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, token)
	bank := industryNamed(t, c, "FSI – Banking")
	hardware := industryNamed(t, c, "Tech - Hardware")
	manual := industryNamed(t, c, "Professional Services")
	bankRoot := industryNamed(t, c, "Financial Services")
	r := a.request("POST", "/v1/candidates", token, map[string]any{"firstName": "Industry Example", "currentCompany": "Example Bank", "currentTitle": "SAP Consultant", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{manual.Id, bank.Id}, "primaryId": bank.Id}})
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var candidate api.Candidate
	r.JSON(&candidate)
	require.NotNil(t, candidate.IndustrySelections)
	assert.Len(t, candidate.IndustrySelections.Ids, 2)
	assert.Equal(t, "Financial Services", candidate.Industry.MustGet())
	// In-use deletion is rejected atomically.
	r = a.request("POST", "/v1/industries/"+bank.Id+"/remove", token, map[string]any{"catalogueVersion": c.Version, "action": "delete"})
	assert.Equal(t, 400, r.Status)
	r = a.request("PATCH", "/v1/industries/"+bankRoot.Id, token, map[string]any{"catalogueVersion": c.Version, "label": "Finance renamed", "aliases": []string{}, "retired": false})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&c)
	r = a.request("GET", "/v1/candidates/"+candidate.Id, token, nil)
	r.JSON(&candidate)
	assert.Equal(t, "Finance renamed", candidate.Industry.MustGet())
	assert.Contains(t, candidate.IndustrySelections.Ids, manual.Id)
	r = a.request("POST", "/v1/industries/"+bank.Id+"/remove", token, map[string]any{"catalogueVersion": c.Version, "action": "delete", "replacementId": hardware.Id})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&c)
	a.request("GET", "/v1/candidates/"+candidate.Id, token, nil).JSON(&candidate)
	assert.ElementsMatch(t, []string{manual.Id, hardware.Id}, candidate.IndustrySelections.Ids)
	require.NotNil(t, candidate.IndustrySelections.PrimaryId)
	assert.Equal(t, hardware.Id, *candidate.IndustrySelections.PrimaryId)
	r = a.request("POST", "/v1/industries/"+hardware.Id+"/remove", token, map[string]any{"catalogueVersion": c.Version, "action": "retire"})
	require.Equal(t, 200, r.Status)
	r.JSON(&c)
	// Retired values remain valid on the existing record but not on a new one.
	choices := map[string]any{"catalogueVersion": c.Version, "ids": []string{hardware.Id}, "primaryId": hardware.Id}
	r = a.request("POST", "/v1/candidates", token, map[string]any{"firstName": "Invalid retired", "industrySelections": choices})
	assert.Equal(t, 400, r.Status)
	r = a.request("PATCH", "/v1/candidates/"+candidate.Id, token, map[string]any{"version": candidate.Version, "industrySelections": choices})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r = a.request("PATCH", "/v1/candidates/"+candidate.Id, tenant.Users[api.RoleViewer].Token, map[string]any{"version": candidate.Version, "industrySelections": choices})
	assert.Equal(t, http.StatusForbidden, r.Status)
}

func TestIndustriesCompanyConcurrencyMoveAndForeignIDs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, token)
	bank := industryNamed(t, c, "FSI – Banking")
	insurance := industryNamed(t, c, "FSI - Insurance")
	education := industryNamed(t, c, "Education")
	r := a.request("POST", "/v1/companies", token, map[string]any{"name": "Example verified employer", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{bank.Id}, "primaryId": bank.Id}})
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var company api.Company
	r.JSON(&company)
	oldVersion := company.Version
	body := map[string]any{"version": oldVersion, "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{insurance.Id}, "primaryId": insurance.Id}}
	r = a.request("PATCH", "/v1/companies/"+company.Id, token, body)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&company)
	assert.Greater(t, company.Version, oldVersion)
	r = a.request("PATCH", "/v1/companies/"+company.Id, token, body)
	assert.Equal(t, 409, r.Status)
	// Moving a child changes the grouping of existing selections, without changing their IDs.
	r = a.request("PATCH", "/v1/industries/"+insurance.Id, token, map[string]any{"catalogueVersion": c.Version, "label": insurance.Label, "aliases": []string{}, "parentId": education.Id, "retired": false})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&c)
	a.request("GET", "/v1/companies/"+company.Id, token, nil).JSON(&company)
	assert.Equal(t, "Education", company.Industry.MustGet())
	assert.Contains(t, company.IndustrySelections.Ids, insurance.Id)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	otherC := industryCatalogue(t, a, other.Users[api.RoleOwner].Token)
	foreign := industryNamed(t, otherC, "FMCG")
	r = a.request("PATCH", "/v1/companies/"+company.Id, token, map[string]any{"version": company.Version, "name": "Must roll back", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{foreign.Id}, "primaryId": foreign.Id}})
	assert.Equal(t, 400, r.Status)
	a.request("GET", "/v1/companies/"+company.Id, token, nil).JSON(&company)
	assert.Equal(t, "Example verified employer", company.Name)
}
func TestIndustriesRecognitionUsesVerifiedEmployerAndSearchesHistoryTags(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, token)
	hardware := industryNamed(t, c, "Tech - Hardware")
	bank := industryNamed(t, c, "FSI – Banking")
	r := a.request("POST", "/v1/companies", token, map[string]any{"name": "Example Systems", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{hardware.Id}, "primaryId": hardware.Id}})
	require.Equal(t, 201, r.Status)
	candidate := a.createCandidate(token, map[string]any{"firstName": "Synthetic Analyst", "currentCompany": "Example Systems", "currentTitle": "SAP Consultant", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{bank.Id}, "primaryId": bank.Id}})
	r = a.request("GET", "/v1/candidates/"+candidate.Id+"/industry-suggestion", token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var suggestion api.IndustrySuggestion
	r.JSON(&suggestion)
	require.NotNil(t, suggestion.CategoryId)
	assert.Equal(t, hardware.Id, *suggestion.CategoryId)
	r = a.request("POST", "/v1/candidates/search", token, map[string]any{"industry": []string{bank.Id}})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	assert.Contains(t, string(r.Body), candidate.Id)
}

func TestIndustriesAutomaticCVImportTracksCatalogueUsage(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	r := a.request("POST", "/v1/cv-imports", token, map[string]any{"source": "linkedin"})
	require.Equal(t, 201, r.Status)
	var imp api.CvImport
	r.JSON(&imp)
	r = a.upload("/v1/cv-imports/"+imp.Id+"/files", token, "", "SyntheticIndustry.pdf", minimalPDF)
	require.Equal(t, 201, r.Status)
	var item api.CvImportItem
	r.JSON(&item)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	text, profile := cvOf("Synthetic Industry Import", "industry-import@example.com", "")
	text += "\nSAP Consultant at Maybank Berhad, 2024 to Present"
	profile.Experience = []api.ParsedJob{{Title: sourced("SAP Consultant", "SAP Consultant"), Company: sourced("Maybank Berhad", "Maybank Berhad"), StartDate: nullable.NewNullableWithValue("2024"), Current: true}}
	readImported(t, a, conn, item.Id, text, profile)
	var candidate api.Candidate
	a.request("GET", "/v1/candidates/"+item.CandidateId.MustGet(), token, nil).JSON(&candidate)
	require.NotNil(t, candidate.IndustrySelections)
	require.Len(t, candidate.IndustrySelections.Ids, 1)
	assert.Equal(t, "Financial Services", candidate.Industry.MustGet())
	c := industryCatalogue(t, a, token)
	bank := industryNamed(t, c, "Financial Services")
	assert.Equal(t, 1, bank.CandidateCount, "imported candidates must count before a category can be deleted")
}

// Saving one candidate must not wait for another candidate's save in the same
// agency: the industry list is only read during a save, so saves share its
// lock. (Each save used to take it exclusively, so every save in an agency
// queued behind the others, and even loading suggestions waited.)
func TestCandidateSavesDoNotQueueBehindEachOther(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	c := a.createCandidate(user.Token, map[string]any{"firstName": "Synthetic Concurrent Save"})
	p := authz.NewPrincipalWithAccess(tenant.ID, user.ID, "", "", api.RoleRecruiter, authz.PermissionsForRole(api.RoleRecruiter), authz.Scope{Users: []string{user.ID}})

	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.InTx(context.Background(), testutil.Current.App, p, func(tx pgx.Tx) error {
			if err := a.modules.Industries.LockIn(context.Background(), tx); err != nil { // another save in progress
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer func() { require.NoError(t, <-done) }()
	defer close(release)

	for name, call := range map[string]func() int{
		"edit another candidate": func() int {
			return a.request(http.MethodPatch, "/v1/candidates/"+c.Id, user.Token, map[string]any{"version": c.Version, "currentTitle": "Synthetic Analyst"}).Status
		},
		"load suggestions": func() int { return a.request(http.MethodGet, "/v1/candidates/used-values", user.Token, nil).Status },
	} {
		status := make(chan int, 1)
		go func() { status <- call() }()
		select {
		case s := <-status:
			assert.Equal(t, http.StatusOK, s, name)
		case <-time.After(3 * time.Second):
			t.Errorf("%s waited for another save to finish", name)
			return
		}
	}
}

// Replacing a category a record already has next to the replacement leaves
// one entry, the current one, and the summary follows it.
func TestIndustryReplacementMergesIntoAnExistingTag(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, token)
	bank, hardware := industryNamed(t, c, "FSI – Banking"), industryNamed(t, c, "Tech - Hardware")
	r := a.request("POST", "/v1/candidates", token, map[string]any{"firstName": "Synthetic Merge Example", "industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{bank.Id, hardware.Id}, "primaryId": bank.Id}})
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var candidate api.Candidate
	r.JSON(&candidate)
	r = a.request("POST", "/v1/industries/"+bank.Id+"/remove", token, map[string]any{"catalogueVersion": c.Version, "action": "delete", "replacementId": hardware.Id})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	a.request("GET", "/v1/candidates/"+candidate.Id, token, nil).JSON(&candidate)
	assert.Equal(t, []string{hardware.Id}, candidate.IndustrySelections.Ids)
	require.NotNil(t, candidate.IndustrySelections.PrimaryId)
	assert.Equal(t, hardware.Id, *candidate.IndustrySelections.PrimaryId)
	assert.Equal(t, industryNamed(t, c, "Information Technology & Services").Label, candidate.Industry.MustGet())
}

// A CV whose jobs have no dates (common) still fills the current title and
// company from the first job listed, as before the industry work; only the
// industry stays unsuggested, since that needs a clearly identified employer.
func TestCVImportFillsCurrentJobWithoutDates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	r := a.request("POST", "/v1/cv-imports", token, map[string]any{"source": "linkedin"})
	require.Equal(t, 201, r.Status)
	var imp api.CvImport
	r.JSON(&imp)
	r = a.upload("/v1/cv-imports/"+imp.Id+"/files", token, "", "SyntheticUndated.pdf", minimalPDF)
	require.Equal(t, 201, r.Status)
	var item api.CvImportItem
	r.JSON(&item)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	text, profile := cvOf("Synthetic Undated Import", "undated-import@example.com", "")
	text += "\nFinance Manager, Example Trading Sdn Bhd\nAccountant, Sample Holdings Berhad"
	profile.Experience = []api.ParsedJob{
		{Title: sourced("Finance Manager", "Finance Manager"), Company: sourced("Example Trading Sdn Bhd", "Example Trading Sdn Bhd")},
		{Title: sourced("Accountant", "Accountant"), Company: sourced("Sample Holdings Berhad", "Sample Holdings Berhad")},
	}
	readImported(t, a, conn, item.Id, text, profile)
	var candidate api.Candidate
	a.request("GET", "/v1/candidates/"+item.CandidateId.MustGet(), token, nil).JSON(&candidate)
	assert.Equal(t, "Finance Manager", candidate.CurrentTitle.MustGet())
	assert.Equal(t, "Example Trading Sdn Bhd", candidate.CurrentCompany.MustGet())
}
