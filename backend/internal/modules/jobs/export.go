package jobs

// Exporting the job list (what its filters find) to Excel or CSV.

import (
	"bytes"
	"context"
	"mime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var jobStatusLabel = map[string]string{"open": "Open", "on_hold": "On hold", "filled": "Filled", "closed": "Closed"}

// Export returns the jobs the filters find as a file.
func (s *Service) Export(ctx context.Context, p authz.Principal, query, status, companyID, contactID, ownerID *string, format string) (sheet.File, error) {
	var found []Summary
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		found, err = sheet.Collect(func(cursor *string) ([]Summary, *string, error) {
			page, err := s.listTx(ctx, tx, query, status, companyID, contactID, ownerID, cursor, 100)
			return page.Items, page.NextCursor, err
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job.exported", EntityType: "job_order", EntityID: p.UserID,
			Changes: map[string]any{"count": len(found), "format": format},
		})
	})
	if err != nil {
		return sheet.File{}, err
	}
	t := sheet.Table{Name: "Jobs", Columns: []sheet.Column{
		{Heading: "Title"}, {Heading: "Client"}, {Heading: "Status"}, {Heading: "Location"}, {Heading: "Openings", Kind: sheet.Number},
		{Heading: "Owner"}, {Heading: "Opened"}, {Heading: "Updated"},
	}}
	for _, j := range found {
		status := jobStatusLabel[j.Status]
		if status == "" {
			status = j.Status
		}
		t.Rows = append(t.Rows, []string{j.Title, j.CompanyName, status, sheet.Str(j.Location), strconv.Itoa(j.Openings),
			sheet.Str(j.OwnerName), sheet.Day(j.CreatedAt), sheet.Day(j.UpdatedAt)})
	}
	return sheet.Write(format, sheet.Dated("Jobs", time.Now()), t)
}

// ExportJobs handles GET /v1/jobs/export.
func (h *HTTP) ExportJobs(ctx context.Context, req api.ExportJobsRequestObject) (api.ExportJobsResponseObject, error) {
	p := authz.MustFromContext(ctx)
	q := req.Params
	var owner *string
	if q.Mine != nil && *q.Mine {
		owner = &p.UserID
	}
	file, err := h.svc.Export(ctx, p, q.Q, c.Convert[string](q.Status), q.CompanyId, q.ContactId, owner, string(q.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportJobs200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportJobs200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
