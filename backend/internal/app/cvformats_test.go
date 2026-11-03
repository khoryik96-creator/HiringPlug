package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var wordText = regexp.MustCompile(`(?s)<w:t(?:\s[^>]*)?>(.*?)</w:t>`)

// docxText is the text of a .docx's main document.
func docxText(t *testing.T, file []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			require.NoError(t, err)
			b, err := io.ReadAll(rc)
			require.NoError(t, err)
			var out []string
			for _, m := range wordText.FindAllStringSubmatch(string(b), -1) {
				out = append(out, m[1])
			}
			return strings.Join(out, "|")
		}
	}
	t.Fatal("not a docx")
	return ""
}

// simpleDocx is a one-paragraph Word file.
func simpleDocx(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func (a *testApp) uploadTemplate(token, name string, file []byte) *response {
	a.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(a.t, w.WriteField("name", name))
	part, err := w.CreateFormFile("file", name+".docx")
	require.NoError(a.t, err)
	_, err = part.Write(file)
	require.NoError(a.t, err)
	require.NoError(a.t, w.Close())
	return a.request(http.MethodPost, "/v1/cv-templates", token, &buf, "Content-Type", w.FormDataContentType())
}

func (a *testApp) formatCv(token, candidateID string, body map[string]any, want int) (api.FormattedCv, string) {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/candidates/"+candidateID+"/formatted-cvs", token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.FormattedCv
	if want != http.StatusCreated {
		return out, ""
	}
	res.JSON(&out)
	file := a.request(http.MethodGet, "/v1/documents/"+out.DocumentId+"/file", token, nil)
	require.Equal(a.t, http.StatusOK, file.Status)
	return out, docxText(a.t, file.Body)
}

func TestFormattedCvs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com", "currentTitle": "Senior Accountant"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/history", recruiter, map[string]any{
		"version":    c.Version,
		"experience": []any{map[string]any{"title": "Senior Accountant", "company": "Maju Bhd", "startDate": "2021-03", "endDate": nil, "current": true, "description": nil}},
		"education":  []any{map[string]any{"qualification": "ACCA", "institution": "ACCA Global", "endYear": 2019}},
	}).Status)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)

	var list api.CvTemplateList
	a.request(http.MethodGet, "/v1/cv-templates", recruiter, nil).JSON(&list)
	assert.Equal(t, "standard", list.Items[0].Id)
	assert.Contains(t, list.Placeholders, "candidate.fullName")

	out, text := a.formatCv(recruiter, c.Id, map[string]any{}, http.StatusCreated)
	assert.Equal(t, "Siti Aisyah - CV (Test tenant TestFormattedCvs).docx", out.FileName)
	assert.Contains(t, text, "Siti Aisyah|Senior Accountant")
	assert.Contains(t, text, "Experience|Senior Accountant|Maju Bhd|Mar 2021 – Present")
	assert.NotContains(t, text, "siti@example.com", "contact hidden unless asked")
	assert.NotContains(t, text, "8,500")
	var doc api.Document
	a.request(http.MethodGet, "/v1/documents/"+out.DocumentId, recruiter, nil).JSON(&doc)
	assert.Equal(t, api.DocumentSourceFormatted, doc.Source, "saved as the formatted CV")

	t.Run("it comes as a PDF too", func(t *testing.T) {
		queued, _ := doc.PdfStatus.Get()
		assert.Equal(t, api.PdfStatusQueued, queued)
		ctx := context.Background()
		pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 test pdf"))
		_, err := testutil.Current.Admin.Exec(ctx, `SELECT job_enqueue($1, 'core', 'document.pdf_rendered', $2::jsonb)`, tenant.ID,
			`{"documentId":"`+out.DocumentId+`","pdfBase64":"`+pdf+`","error":null}`)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		a.request(http.MethodGet, "/v1/documents/"+out.DocumentId, recruiter, nil).JSON(&doc)
		status, _ := doc.PdfStatus.Get()
		assert.Equal(t, api.PdfStatusReady, status)
		res := a.request(http.MethodGet, "/v1/documents/"+out.DocumentId+"/pdf", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status)
		assert.Equal(t, "%PDF-1.7 test pdf", string(res.Body))
		assert.Contains(t, res.Header.Get("Content-Disposition"), "Siti Aisyah - CV")
	})

	t.Run("contact and salary when asked, salary only for those who may see it", func(t *testing.T) {
		_, text := a.formatCv(recruiter, c.Id, map[string]any{"showContact": true, "includeSalary": true, "summary": "Strong IFRS."}, http.StatusCreated)
		assert.Contains(t, text, "Email: siti@example.com")
		assert.Contains(t, text, "Current salary: MYR 8,500 a month")
		assert.Contains(t, text, "Summary|Strong IFRS.")
		a.formatCv(sourcer, c.Id, map[string]any{"includeSalary": true}, http.StatusForbidden)
		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT string_agg(changes::text, ' ') FROM audit_log WHERE entity_id = $1 AND action = 'cv.formatted'`, c.Id).Scan(&changes))
		assert.NotContains(t, changes, "8500")
	})

	t.Run("for a job: its title and client", func(t *testing.T) {
		company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
		job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Finance Manager"})
		app := a.addToJob(recruiter, job.Id, c.Id)
		_, text := a.formatCv(recruiter, c.Id, map[string]any{"applicationId": app.Id}, http.StatusCreated)
		assert.Contains(t, text, "Position: Finance Manager|Client: Contoh Berhad")
		other := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Wei"}).Id)
		a.formatCv(recruiter, c.Id, map[string]any{"applicationId": other.Id}, http.StatusBadRequest)
	})

	t.Run("blind: no name, contact or nationality, and checked before saving", func(t *testing.T) {
		out, text := a.formatCv(recruiter, c.Id, map[string]any{"blind": true, "showContact": true, "summary": "Siti is strong in IFRS."}, http.StatusCreated)
		assert.NotContains(t, out.FileName, "Siti")
		assert.Contains(t, out.FileName, "Blind CV")
		assert.Contains(t, text, "Candidate C-")
		assert.NotContains(t, text, "Siti")
		assert.NotContains(t, text, "Aisyah")
		assert.NotContains(t, text, "siti@example.com", "blind wins over show contact")
		assert.Contains(t, text, "the candidate is strong in IFRS.")

		res := a.uploadTemplate(recruiter, "Named", simpleDocx(t, "CV of Siti Aisyah: {{candidate.fullName}}"))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var named api.CvTemplate
		res.JSON(&named)
		res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", recruiter, map[string]any{"templateId": named.Id, "blind": true})
		assert.Equal(t, api.ErrorCodeCVNOTBLIND, res.Error().Error.Code, "the template's own text gives the name away: not saved")
	})

	t.Run("the company's own template", func(t *testing.T) {
		res := a.uploadTemplate(recruiter, "Branded", simpleDocx(t, "CONTOH CV: {{candidate.fullName}} / {{candidate.salary}}"))
		assert.Equal(t, []string{"file"}, fieldNames(t, res.Error()), "unknown placeholders are refused")
		res = a.uploadTemplate(recruiter, "Not Word", []byte("%PDF-1.7 not a docx"))
		assert.Equal(t, http.StatusUnsupportedMediaType, res.Status)
		assert.Equal(t, http.StatusForbidden, a.uploadTemplate(sourcer, "Mine", simpleDocx(t, "{{candidate.fullName}}")).Status)

		res = a.uploadTemplate(recruiter, "Branded", simpleDocx(t, "CONTOH CV: {{candidate.fullName}} ({{candidate.initials}})"))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var tpl api.CvTemplate
		res.JSON(&tpl)
		_, text := a.formatCv(recruiter, c.Id, map[string]any{"templateId": tpl.Id}, http.StatusCreated)
		assert.Equal(t, "CONTOH CV: Siti Aisyah (S.A.)", text)

		std := a.request(http.MethodGet, "/v1/cv-templates/standard/file", recruiter, nil)
		require.Equal(t, http.StatusOK, std.Status)
		assert.Contains(t, docxText(t, std.Body), "{{candidate.fullName}}", "the standard template, to start your own from")
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/cv-templates/"+tpl.Id, recruiter, nil).Status)
		a.formatCv(recruiter, c.Id, map[string]any{"templateId": tpl.Id}, http.StatusNotFound)
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		a.formatCv(stranger, c.Id, map[string]any{}, http.StatusNotFound)
	})
}
