package auditlog

// The Excel and CSV downloads of both logs (what the filters find, at most
// sheet.MaxRows), each audited. Neither holds an amount.

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

// exportPage is how many rows each read of an export takes.
const exportPage = 500

// ExportLog returns the audit log the filter finds as a file.
func (s *Service) ExportLog(ctx context.Context, p authz.Principal, f LogFilter, format string) (sheet.File, error) {
	var entries []Entry
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		af := s.auditFilter(f)
		var err error
		entries, err = sheet.Collect(func(cursor *string) ([]Entry, *string, error) {
			page, err := s.logPage(ctx, tx, af, cursor, exportPage)
			return page.Items, page.NextCursor, err
		})
		if err != nil {
			return err
		}
		return exported(ctx, tx, p, "audit_log", len(entries), format)
	})
	if err != nil {
		return sheet.File{}, err
	}
	return sheet.Write(format, sheet.Dated("Audit log", s.now(), s.loc), LogTable(entries, s.loc))
}

// ExportSalaryViews returns the salary views the filter finds as a file.
func (s *Service) ExportSalaryViews(ctx context.Context, p authz.Principal, f ViewFilter, format string) (sheet.File, error) {
	var views []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		views, err = sheet.Collect(func(cursor *string) ([]View, *string, error) {
			page, err := s.viewPage(ctx, tx, f, cursor, exportPage)
			return page.Items, page.NextCursor, err
		})
		if err != nil {
			return err
		}
		return exported(ctx, tx, p, "salary_views", len(views), format)
	})
	if err != nil {
		return sheet.File{}, err
	}
	return sheet.Write(format, sheet.Dated("Salary views", s.now(), s.loc), ViewTable(views, s.loc))
}

func exported(ctx context.Context, tx pgx.Tx, p authz.Principal, list string, count int, format string) error {
	return audit.Record(ctx, tx, audit.Entry{
		Action: "auditlog.exported", EntityType: "user", EntityID: p.UserID,
		Changes: map[string]any{"list": list, "count": count, "format": format},
	})
}

// moment writes a time in the agency's time zone, to the minute.
func moment(t time.Time, loc *time.Location) string { return t.In(loc).Format("2006-01-02 15:04") }

// LogTable is the audit log export's sheet.
func LogTable(entries []Entry, loc *time.Location) sheet.Table {
	t := sheet.Table{Name: "Audit log", Columns: []sheet.Column{
		{Heading: "When"}, {Heading: "Who"}, {Heading: "Action"}, {Heading: "Record type"}, {Heading: "Record"}, {Heading: "Record id"}, {Heading: "Changes"},
	}, Rows: make([][]string, 0, len(entries))}
	for _, e := range entries {
		changes := ""
		if len(e.Changes) > 0 {
			if raw, err := json.Marshal(e.Changes); err == nil {
				changes = FitCell(string(raw))
			}
		}
		t.Rows = append(t.Rows, []string{moment(e.CreatedAt, loc), e.ActorName, e.Action, e.EntityType, sheet.Str(e.EntityName), e.EntityID, changes})
	}
	return t
}

// ViewTable is the salary views export's sheet.
func ViewTable(views []View, loc *time.Location) sheet.Table {
	t := sheet.Table{Name: "Salary views", Columns: []sheet.Column{
		{Heading: "Day"}, {Heading: "Who"}, {Heading: "Where"}, {Heading: "Candidate"}, {Heading: "Candidate id"},
		{Heading: "Candidates", Kind: sheet.Number}, {Heading: "Times", Kind: sheet.Number}, {Heading: "First"}, {Heading: "Last"},
	}, Rows: make([][]string, 0, len(views))}
	for _, v := range views {
		where := KindLabels[v.Kind]
		if where == "" {
			where = v.Kind
		}
		t.Rows = append(t.Rows, []string{
			sheet.Day(v.Day), v.UserName, where, sheet.Str(v.CandidateName), sheet.Str(v.CandidateID),
			strconv.Itoa(v.Candidates), strconv.Itoa(v.Count), moment(v.FirstAt, loc), moment(v.LastAt, loc),
		})
	}
	return t
}
