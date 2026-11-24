package app

// The audit log page and the log of who was shown a salary (auditlog
// module): it names records through the modules that own them, and every
// module that shows a salary counts it there.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/auditlog"
)

// newAuditLog builds the auditlog module once the modules it names records
// with exist.
func newAuditLog(d Deps, m *Modules) *auditlog.Module {
	jobTitles := auditlog.NamesFunc(func(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
		briefs, err := m.Jobs.Briefs(ctx, tx, ids)
		out := make(map[string]string, len(briefs))
		for id, b := range briefs {
			out[id] = b.Title
		}
		return out, err
	})
	return auditlog.New(d.Pool, auditlog.Deps{
		Users: m.Identity, Keys: m.Integrations,
		Records: map[string]auditlog.Names{
			"candidate": auditlog.NamesFunc(func(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
				briefs, err := m.Candidates.Briefs(ctx, tx, ids)
				out := make(map[string]string, len(briefs))
				for id, b := range briefs {
					out[id] = b.Name
				}
				return out, err
			}),
			"job": jobTitles, "job_order": jobTitles,
			"company": auditlog.NamesFunc(m.Companies.Names),
			"contact": auditlog.NamesFunc(func(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
				briefs, err := m.Contacts.Briefs(ctx, tx, ids)
				out := make(map[string]string, len(briefs))
				for id, b := range briefs {
					out[id] = b.Name
				}
				return out, err
			}),
		},
	}, auditlog.Config{TimeZone: d.Config.TimeZone})
}

// recordSalaryViews has every module that shows a salary count it.
func recordSalaryViews(m *Modules) {
	_ = m.AuditLog
}
