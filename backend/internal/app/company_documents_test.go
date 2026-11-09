package app_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// addDocument uploads a document to a client's terms or benefits.
func (a *testApp) addDocument(token, companyID, section, fileName string, content []byte) *response {
	a.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(a.t, w.WriteField("section", section))
	part, err := w.CreateFormFile("file", fileName)
	require.NoError(a.t, err)
	_, err = part.Write(content)
	require.NoError(a.t, err)
	require.NoError(a.t, w.Close())
	return a.request(http.MethodPost, "/v1/companies/"+companyID+"/documents", token, &buf, "Content-Type", w.FormDataContentType())
}

func TestCompanyDocumentsAndWhoMayDeleteThem(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleViewer)
	owner, admin := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token
	recruiter, viewer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	path := "/v1/companies/" + company.Id + "/documents"

	res := a.addDocument(recruiter, company.Id, "terms", "Signed TOB 2026.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var terms api.CompanyDocument
	res.JSON(&terms)
	assert.Equal(t, "Signed TOB 2026.pdf", terms.FileName)
	assert.Equal(t, api.CompanyDocumentSectionTerms, terms.Section)
	assert.Equal(t, "application/pdf", terms.ContentType)
	assert.Equal(t, len(minimalPDF), terms.SizeBytes)
	assert.NotEmpty(t, terms.CreatedByName.MustGet())

	res = a.addDocument(recruiter, company.Id, "benefits", "brochure.txt", []byte("Medical: GHS for staff and family"))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var brochure api.CompanyDocument
	res.JSON(&brochure)

	t.Run("bad uploads are refused", func(t *testing.T) {
		assert.Equal(t, []string{"section"}, fieldNames(t, a.addDocument(recruiter, company.Id, "offer", "x.pdf", minimalPDF).Error()))
		assert.Equal(t, http.StatusBadRequest, a.addDocument(recruiter, company.Id, "terms", "x.exe", []byte{0x4d, 0x5a, 0x90, 0x00}).Status)
		assert.Equal(t, http.StatusForbidden, a.addDocument(viewer, company.Id, "terms", "x.pdf", minimalPDF).Status, "viewers only read")
		assert.Equal(t, http.StatusNotFound, a.addDocument(recruiter, "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", "terms", "x.pdf", minimalPDF).Status)
	})

	var list api.CompanyDocumentList
	res = a.request(http.MethodGet, path, viewer, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	require.Len(t, list.Items, 2)
	assert.Equal(t, terms.Id, list.Items[0].Id)

	res = a.request(http.MethodGet, path+"/"+terms.Id+"/file", viewer, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, minimalPDF, res.Body)
	assert.Contains(t, res.Header.Get("Content-Disposition"), "Signed TOB 2026.pdf")

	t.Run("deleting is a permission of its own: owners and admins to start with", func(t *testing.T) {
		assert.Contains(t, authz.PermissionsForRole(api.RoleAdmin), api.PermissionCompaniesDocumentsDelete)
		assert.NotContains(t, a.me(recruiter).Permissions, api.PermissionCompaniesDocumentsDelete)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, path+"/"+brochure.Id, recruiter, nil).Status)

		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, path+"/"+brochure.Id, admin, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path+"/"+brochure.Id+"/file", viewer, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, path+"/"+brochure.Id, admin, nil).Status)

		// The owner passes it to a recruiter they trust.
		trusted := append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionCompaniesDocumentsDelete)
		a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": trusted, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, path+"/"+terms.Id, recruiter, nil).Status)
		a.request(http.MethodGet, path, viewer, nil).JSON(&list)
		assert.Empty(t, list.Items)
	})

	t.Run("another company's document is not found through this one", func(t *testing.T) {
		other := a.createCompany(recruiter, map[string]any{"name": "Contoh Sdn Bhd"})
		res := a.addDocument(recruiter, other.Id, "terms", "tob.pdf", minimalPDF)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var doc api.CompanyDocument
		res.JSON(&doc)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path+"/"+doc.Id+"/file", viewer, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, path+"/"+doc.Id, owner, nil).Status)
	})
}
