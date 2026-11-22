package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// csvRows reads an exported CSV (after its byte-order mark).
func csvRows(t *testing.T, res *response) [][]string {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	require.True(t, bytes.HasPrefix(res.Body, []byte("\xEF\xBB\xBF")))
	r := csv.NewReader(bytes.NewReader(res.Body[3:]))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	require.NoError(t, err)
	return rows
}

// xlsxRows reads the cells of an exported workbook's sheets, in order.
func xlsxRows(t *testing.T, res *response) map[string][][]string {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	z, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
	require.NoError(t, err)
	read := func(name string) []byte {
		f, err := z.Open(name)
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		return b
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
		} `xml:"sheets>sheet"`
	}
	require.NoError(t, xml.Unmarshal(read("xl/workbook.xml"), &wb))
	out := map[string][][]string{}
	for i, s := range wb.Sheets {
		var ws struct {
			Rows []struct {
				Cells []struct {
					Ref    string `xml:"r,attr"`
					Value  string `xml:"v"`
					Inline string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		require.NoError(t, xml.Unmarshal(read("xl/worksheets/sheet"+string(rune('1'+i))+".xml"), &ws))
		for _, r := range ws.Rows {
			var row []string
			for _, c := range r.Cells {
				col := 0 // the cell's column, from its letters (empty cells are left out)
				for _, ch := range strings.TrimRight(c.Ref, "0123456789") {
					col = col*26 + int(ch-'A'+1)
				}
				for len(row) < col-1 {
					row = append(row, "")
				}
				row = append(row, c.Inline+c.Value)
			}
			out[s.Name] = append(out[s.Name], row)
		}
	}
	return out
}

func column(rows [][]string, heading string) int {
	for i, h := range rows[0] {
		if h == heading {
			return i
		}
	}
	return -1
}

func TestCandidateExport(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah", "email": "siti@example.com", "tags": []string{"finance"}})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+siti.Id+"/compensation", owner, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	a.createCandidate(recruiter, map[string]any{"firstName": "=HYPERLINK(\"http://x\")", "lastName": "Tan"})
	a.createCandidate(recruiter, map[string]any{"firstName": "Wei", "lastName": "Ming"})

	res := a.request(http.MethodPost, "/v1/candidates/export", admin, map[string]any{"format": "csv"})
	rows := csvRows(t, res)
	assert.Contains(t, res.Header.Get("Content-Disposition"), `filename="Candidates `)
	require.Len(t, rows, 4, "a heading and everyone")
	name, salary := column(rows, "Name"), column(rows, "Current salary")
	require.NotEqual(t, -1, salary, "an admin may see salaries")
	var names []string
	for _, r := range rows[1:] {
		names = append(names, r[name])
		if r[name] == "Siti Aminah" {
			assert.Equal(t, "8500.00", r[salary])
			assert.Equal(t, "finance", r[column(rows, "Tags")])
		}
	}
	assert.Contains(t, names, `'=HYPERLINK("http://x") Tan`, "a name that looks like a formula is written as text")

	found := csvRows(t, a.request(http.MethodPost, "/v1/candidates/export", admin, map[string]any{"format": "csv", "search": map[string]any{"q": "Siti", "limit": 1}}))
	require.Len(t, found, 2, "only what the search finds (its limit ignored)")
	assert.Equal(t, "siti@example.com", found[1][column(found, "Email")])

	for _, role := range []api.Role{api.RoleRecruiter, api.RoleSourcer} {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/candidates/export", tenant.Users[role].Token, map[string]any{"format": "csv"}).Status,
			"only owners and admins export candidates")
	}

	// A role that may export but not see salaries gets no salary columns.
	var roles api.AccessSettings
	a.request(http.MethodGet, "/v1/access", owner, nil).JSON(&roles)
	var rec api.RoleAccess
	for _, r := range roles.Roles {
		if r.Role == "admin" {
			rec = r
		}
	}
	a.setRole(owner, "admin", map[string]any{"version": rec.Version, "permissions": without(rec.Permissions, api.PermissionCandidatesCompensationRead, api.PermissionCandidatesCompensationUpdate),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	book := xlsxRows(t, a.request(http.MethodPost, "/v1/candidates/export", admin, map[string]any{"format": "xlsx"}))
	require.Len(t, book["Candidates"], 4)
	assert.Equal(t, -1, column(book["Candidates"], "Current salary"))
	assert.NotContains(t, strings.Join(book["Candidates"][0], ","), "salary")

	var changes string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT changes::text FROM audit_log WHERE tenant_id = $1 AND action = 'candidate.exported' ORDER BY created_at LIMIT 1`, tenant.ID).Scan(&changes))
	assert.Contains(t, changes, `"count": 3`)
	assert.NotContains(t, changes, "8500", "never a salary in the audit log")
}

func TestListExports(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter, api.RoleViewer)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	acme := a.createCompany(recruiter, map[string]any{"name": "Acme Sdn Bhd", "city": "Petaling Jaya"})
	a.createCompany(recruiter, map[string]any{"name": "Kenanga Logistics Sdn Bhd"})
	a.createContact(recruiter, map[string]any{"companyId": acme.Id, "firstName": "Aisyah", "lastName": "Rahman", "email": "aisyah@acme.example", "role": "hr"})
	job := a.createJob(recruiter, map[string]any{"companyId": acme.Id, "title": "Finance Manager"})
	app := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti"}).Id)
	a.placement(http.MethodPost, "/v1/applications/"+app.Id+"/placement", recruiter, map[string]any{
		"startDate": "2026-12-01", "money": map[string]any{"salary": map[string]any{"amount": "9000.00", "currency": "MYR", "period": "month"}, "feePercent": "20"},
	}, http.StatusCreated)

	companies := xlsxRows(t, a.request(http.MethodGet, "/v1/companies/export?format=xlsx&q=acme", admin, nil))["Companies"]
	require.Len(t, companies, 2, "the search is kept")
	assert.Equal(t, "Acme Sdn Bhd", companies[1][0])
	assert.Equal(t, "Client", companies[1][column(companies, "Type")])

	contacts := csvRows(t, a.request(http.MethodGet, "/v1/contacts/export?format=csv&companyId="+acme.Id, admin, nil))
	require.Len(t, contacts, 2)
	assert.Equal(t, []string{"Aisyah Rahman", "Acme Sdn Bhd", "HR", "aisyah@acme.example"},
		[]string{contacts[1][0], contacts[1][column(contacts, "Company")], contacts[1][column(contacts, "Role")], contacts[1][column(contacts, "Email")]})

	jobs := csvRows(t, a.request(http.MethodGet, "/v1/jobs/export?format=csv&status=open", recruiter, nil))
	require.Len(t, jobs, 2)
	assert.Equal(t, "Finance Manager", jobs[1][0])
	assert.Equal(t, "Acme Sdn Bhd", jobs[1][column(jobs, "Client")])

	placements := xlsxRows(t, a.request(http.MethodGet, "/v1/placements/export?format=xlsx", admin, nil))["Placements"]
	require.Len(t, placements, 2)
	assert.Equal(t, "9000.00", placements[1][column(placements, "Salary")])
	assert.Equal(t, "2026-12-01", placements[1][column(placements, "Start date")])

	report := xlsxRows(t, a.request(http.MethodGet, "/v1/reports/summary/export?format=xlsx", recruiter, nil))
	for _, name := range []string{"Pipeline", "Sources", "Recruiters", "Time to fill"} {
		assert.Contains(t, report, name, "a sheet each")
	}
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/reports/summary/export?format=csv", recruiter, nil).Status)

	viewer := tenant.Users[api.RoleViewer].Token
	for _, path := range []string{"/v1/companies/export?format=csv", "/v1/contacts/export?format=csv", "/v1/jobs/export?format=csv", "/v1/placements/export?format=csv"} {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, path, viewer, nil).Status, path)
	}
	for _, path := range []string{"/v1/companies/export?format=csv", "/v1/contacts/export?format=csv", "/v1/placements/export?format=csv"} {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, path, recruiter, nil).Status, "owners and admins only: "+path)
	}
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/companies/export?format=pdf", admin, nil).Status)
}

func TestExportsHoldAtMostTenThousandRows(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO company (tenant_id, name, created_by)
		SELECT $1, 'Company ' || n, $2 FROM generate_series(1, 10001) n`, tenant.ID, tenant.Users[api.RoleAdmin].ID)
	require.NoError(t, err)
	res := a.request(http.MethodGet, "/v1/companies/export?format=csv", tenant.Users[api.RoleAdmin].Token, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
	assert.Contains(t, string(res.Body), "EXPORT_TOO_LARGE")
}
