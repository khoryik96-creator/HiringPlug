package candidates

// Exporting what a search finds to Excel or CSV. Salaries are in it only
// for roles that may see them (and never in the audit entry); a search that
// finds more than sheet.MaxRows must be narrowed.

import (
	"bytes"
	"context"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var availabilityLabel = map[string]string{"looking": "Looking out", "passive": "Passive", "placed": "Placed", "not_looking": "Not looking"}

var workAuthLabel = map[string]string{
	"local": "Local", "expat_eligible": "Expat, can work here", "requires_visa": "Requires a visa",
	"rp_t": "Residence Pass-Talent (RP-T)", "permanent_resident": "Permanent resident", "spouse_visa": "Spouse visa", "other": "Other pass",
}

// exportPage is how many an export reads at a time (more than a screen's
// page: each read runs the whole search again).
const exportPage = 1000

// Export returns everyone the search finds as a file.
func (s *Service) Export(ctx context.Context, p authz.Principal, f SearchFilter, format string) (sheet.File, error) {
	f.NoSnippets = true
	var found []Summary
	var labels map[string]map[string]string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		if s.labels != nil {
			if labels, err = s.labels.Labels(ctx, tx); err != nil {
				return err
			}
		}
		var cursor *string
		limit := exportPage
		if f.Meaning != nil { // the best matches the page shows, best first
			limit = MeaningShown
		}
		for {
			page, err := s.searchTx(ctx, tx, p, f, cursor, limit)
			if err != nil {
				return err
			}
			if err := meaningReady(page.Meaning); err != nil {
				return err
			}
			for _, hit := range page.Items {
				found = append(found, hit.Summary)
			}
			if len(found) > sheet.MaxRows {
				return apperr.ExportTooLarge(sheet.MaxRows)
			}
			if page.NextCursor == nil && page.NextOffset == nil {
				break
			}
			cursor = page.NextCursor
			if page.NextOffset != nil {
				f.Order.Offset = *page.NextOffset
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.exported", EntityType: "user", EntityID: p.UserID,
			Changes: map[string]any{"count": len(found), "format": format, "salaries": canSeeSalary(p)},
		})
	})
	if err != nil {
		return sheet.File{}, err
	}
	return sheet.Write(format, sheet.Dated("Candidates", time.Now(), s.zone), CandidateTable(found, canSeeSalary(p), labels["candidate_source"], s.zone))
}

// CandidateTable is the export's sheet: the salary columns only when shown,
// sources in words, the day updated in the agency's time zone.
func CandidateTable(found []Summary, salaries bool, sources map[string]string, loc *time.Location) sheet.Table {
	cols := []sheet.Column{
		{Heading: "Name"}, {Heading: "Email"}, {Heading: "Phone"}, {Heading: "Current title"}, {Heading: "Current company"},
		{Heading: "Location"}, {Heading: "Nationality"}, {Heading: "Country of residence"}, {Heading: "Work authorization"},
		{Heading: "Availability"}, {Heading: "Notice period"}, {Heading: "Source"}, {Heading: "Industry"}, {Heading: "Tags"},
	}
	if salaries {
		cols = append(cols, sheet.Column{Heading: "Current salary", Kind: sheet.Number}, sheet.Column{Heading: "Current salary currency"},
			sheet.Column{Heading: "Current salary per"}, sheet.Column{Heading: "Expected salary from", Kind: sheet.Number},
			sheet.Column{Heading: "Expected salary to", Kind: sheet.Number}, sheet.Column{Heading: "Expected salary currency"},
			sheet.Column{Heading: "Expected salary per"})
	}
	cols = append(cols, sheet.Column{Heading: "Updated"})
	t := sheet.Table{Name: "Candidates", Columns: cols, Rows: make([][]string, 0, len(found))}
	for _, c := range found {
		auth := label(workAuthLabel, c.WorkAuthorizationStatus)
		if basis := label(workAuthLabel, c.WorkAuthorizationBasis); basis != "" {
			auth = strings.TrimPrefix(auth+", "+basis, ", ")
		}
		row := []string{
			c.FullName, str(c.Email), str(c.Phone), str(c.CurrentTitle), str(c.CurrentCompany), str(c.Location),
			str(c.Nationality), str(c.ResidenceCountry), auth, label(availabilityLabel, c.Availability), noticeText(c.NoticePeriod),
			sheet.Label(sources, c.Source), str(c.Industry), strings.Join(c.Tags, ", "),
		}
		if salaries {
			row = append(row, salaryCells(c.Compensation)...)
		}
		t.Rows = append(t.Rows, append(row, sheet.DayIn(c.UpdatedAt, loc)))
	}
	return t
}

func salaryCells(c *Compensation) []string {
	out := make([]string, 7)
	if c == nil {
		return out
	}
	if a := c.Current; a != nil {
		out[0], out[1], out[2] = a.Amount, a.Currency, a.Period
	}
	if e := c.Expected; e != nil {
		out[3], out[5], out[6] = e.Min, e.Currency, e.Period
		out[4] = str(e.Max)
	}
	return out
}

func noticeText(n *NoticePeriod) string {
	if n == nil {
		return ""
	}
	unit := n.Unit
	if n.Value != 1 {
		unit += "s"
	}
	return strconv.Itoa(n.Value) + " " + unit
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func label(labels map[string]string, v *string) string {
	if v == nil {
		return ""
	}
	if l, ok := labels[*v]; ok {
		return l
	}
	return *v
}

// ExportCandidates handles POST /v1/candidates/export.
func (h *HTTP) ExportCandidates(ctx context.Context, req api.ExportCandidatesRequestObject) (api.ExportCandidatesResponseObject, error) {
	p := authz.MustFromContext(ctx)
	var search api.CandidateSearch
	if req.Body.Search != nil {
		search = *req.Body.Search
	}
	f := FilterOf(search, p)
	if search.Sort != nil {
		f.Order.Sort = string(*search.Sort)
	}
	f.Order.Offset = 0
	file, err := h.svc.Export(ctx, p, f, string(req.Body.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportCandidates200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportCandidates200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
