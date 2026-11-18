package main

import (
	_ "embed"
	"encoding/csv"
	"errors"
	"io"
	"strings"
)

//go:embed extraction_scope.csv
var extractionScope string

type RouteCoverage struct {
	Path       string `json:"path"`
	Family     string `json:"family"`
	Policy     string `json:"policy"`
	Equivalent string `json:"equivalent,omitempty"`
	Requests   int64  `json:"requests"`
	Collected  int64  `json:"collected"`
	Gaps       int64  `json:"gaps"`
	Rows       int64  `json:"rows"`
}
type ScopeCoverage struct {
	Area      string `json:"area"`
	Required  string `json:"required"`
	Sources   string `json:"sources"`
	Status    string `json:"status"`
	Collected int64  `json:"collected"`
	Gaps      int64  `json:"gaps"`
}
type ExtractionReport struct {
	Summary          ExtractionSummary `json:"summary"`
	Routes           []RouteCoverage   `json:"routes"`
	Scope            []ScopeCoverage   `json:"scope"`
	Imports          []SourceImport    `json:"imports"`
	UnlinkedObjects  int64             `json:"unlinkedObjects"`
	VerifiedComplete bool              `json:"verifiedComplete"`
	Limitations      []string          `json:"limitations"`
}

func scopeAreas(r ExtractRoute) []int {
	areas := []int{29}
	p := r.Path
	if strings.Contains(p, "/deleted") {
		return append(areas, 28)
	}
	if strings.Contains(p, "/attachments") {
		areas = append(areas, 5, 6)
	}
	if strings.Contains(p, "/photo") || strings.Contains(p, "/logo") || strings.Contains(p, "/videos") {
		areas = append(areas, 6)
	}
	if strings.Contains(p, "/skills") {
		areas = append(areas, 1)
	}
	if strings.Contains(p, "/notes") || r.Root == "notes" {
		areas = append(areas, 2, 3, 4)
	}
	if strings.Contains(p, "/fields/") || strings.Contains(p, "/lists/") {
		areas = append(areas, 23)
	}
	if strings.Contains(p, "/activities") || strings.Contains(p, "/activitysettings") {
		areas = append(areas, 15)
	}
	if strings.Contains(p, "/invoices") {
		areas = append(areas, 13)
	}
	if strings.Contains(p, "/timesheets") || strings.Contains(p, "/payrollbilling") || strings.Contains(p, "/ratecard") {
		areas = append(areas, 14)
	}
	if strings.Contains(p, "/history") {
		areas = append(areas, 28)
	}
	switch r.Root {
	case "candidates":
		areas = append(areas, 0, 1, 23, 24)
	case "companies":
		areas = append(areas, 7, 23)
	case "contacts":
		areas = append(areas, 8, 23, 24)
	case "jobs":
		areas = append(areas, 9, 23)
	case "applications":
		areas = append(areas, 10, 23)
	case "submissions", "floats":
		areas = append(areas, 11)
	case "placements":
		areas = append(areas, 12, 23)
	case "interviews", "events":
		areas = append(areas, 16)
	case "usertasks":
		areas = append(areas, 17)
	case "folders":
		areas = append(areas, 18)
	case "jobads", "jobboards":
		areas = append(areas, 19)
	case "requisitions":
		areas = append(areas, 20, 23)
	case "opportunities":
		areas = append(areas, 21)
	case "users", "usergroups", "useroffices":
		areas = append(areas, 22)
	case "categories", "countries", "locations", "worktypes":
		areas = append(areas, 1, 23)
	case "partners", "webhooks":
		areas = append(areas, 27)
	}
	unique := map[int]bool{}
	out := []int{}
	for _, a := range areas {
		if !unique[a] {
			unique[a] = true
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) extractionReport() (ExtractionReport, error) {
	var report ExtractionReport
	var err error
	if report.Summary, err = s.ExtractionStatus(); err != nil {
		return report, err
	}
	if report.Imports, err = s.sourceImports(); err != nil {
		return report, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM blobs WHERE id NOT IN (SELECT blob_id FROM extract_fetches UNION SELECT blob_id FROM source_imports UNION SELECT blob_id FROM source_members)`).Scan(&report.UnlinkedObjects); err != nil {
		return report, err
	}
	reader := csv.NewReader(strings.NewReader(extractionScope))
	if _, err = reader.Read(); err != nil {
		return report, err
	}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return report, err
		}
		report.Scope = append(report.Scope, ScopeCoverage{Area: row[0], Required: row[1], Sources: row[2], Status: "not_started"})
	}
	if len(report.Scope) != 30 {
		return report, errors.New("invalid bundled scope catalogue")
	}
	counts := map[string]RouteCoverage{}
	rows, err := s.db.Query(`SELECT route,COUNT(*),COALESCE(SUM(status='collected'),0),COALESCE(SUM(status NOT IN ('pending','running','collected')),0),COALESCE(SUM(rows),0) FROM extract_tasks WHERE extraction_id=? GROUP BY route`, report.Summary.ID)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var c RouteCoverage
		if err := rows.Scan(&c.Path, &c.Requests, &c.Collected, &c.Gaps, &c.Rows); err != nil {
			rows.Close()
			return report, err
		}
		counts[c.Path] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	for _, r := range extractionRoutes() {
		c := counts[r.Path]
		c.Path = r.Path
		c.Family = r.Family
		c.Policy = "Read from API when actual source identifiers are available"
		if c.Equivalent = equivalentSource(r); c.Equivalent != "" {
			c.Policy = "Collected through paged bulk source; source access and reconciliation still apply"
		}
		if excluded := routeExcluded(r); excluded != "" {
			c.Policy = excluded
		}
		report.Routes = append(report.Routes, c)
		if c.Requests > 0 {
			for _, index := range scopeAreas(r) {
				report.Scope[index].Collected += c.Collected
				report.Scope[index].Gaps += c.Gaps
			}
		}
	}
	for i := range report.Scope {
		area := &report.Scope[i]
		switch {
		case area.Gaps > 0:
			area.Status = "api_requests_unresolved"
		case area.Collected > 0:
			area.Status = "api_data_saved_unverified"
		case report.Summary.ID > 0:
			area.Status = "no_api_evidence_yet"
		}
		if i == 3 || i == 4 || i == 24 || i == 25 || i == 26 || i == 28 {
			area.Status = "provider_export_required"
		}
	}
	report.Limitations = []string{
		"All completed saved objects, including unlinked objects from interrupted indexing, are exported. Unlinked objects require association review.",
		"API pass finished means the known request queue was processed; it is not proof that all account data was recovered.",
		"Full emails, provider communications, personal settings, templates, privacy and audit history need provider/export reconciliation. Notes are only partial evidence.",
		"Recently deleted endpoints retain limited deletion markers, not deleted record bodies. Purged data cannot be recreated.",
		"Offset pagination is not a snapshot. Re-run a new extraction at cutover and reconcile source IDs, counts, links and files against provider totals.",
		"A zero-request child route has no evidence: its parent may be empty, inaccessible or missing a required identifier. Review route coverage and unresolved tasks in the manifest.",
		"Uploaded originals are retained unchanged. Generic row indexing does not verify source identity, interpret unknown schemas or make a SQL/PST backup readable.",
		"External video URLs and rate-card references are retained as references; their external contents require a separate authorized source. API photos may be resized.",
	}
	return report, nil
}
