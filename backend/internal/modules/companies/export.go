package companies

// Exporting the company list (what its filters find) to Excel or CSV.

import (
	"bytes"
	"context"
	"mime"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var typeLabel = map[string]string{"client": "Client", "prospect": "Prospect", "internal": "Internal"}

// Export returns the companies the filter finds as a file.
func (s *Service) Export(ctx context.Context, p authz.Principal, f ListFilter, format string) (sheet.File, error) {
	var found []Summary
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		found, err = sheet.Collect(func(cursor *string) ([]Summary, *string, error) {
			page, err := s.listTx(ctx, tx, p, f, cursor, 100)
			return page.Items, page.NextCursor, err
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "company.exported", EntityType: "company", EntityID: p.UserID,
			Changes: map[string]any{"count": len(found), "format": format},
		})
	})
	if err != nil {
		return sheet.File{}, err
	}
	t := sheet.Table{Name: "Companies", Columns: []sheet.Column{
		{Heading: "Name"}, {Heading: "Type"}, {Heading: "Category"}, {Heading: "Industry"}, {Heading: "City"},
		{Heading: "Website"}, {Heading: "LinkedIn"}, {Heading: "Updated"},
	}}
	for _, c := range found {
		kind := typeLabel[c.Type]
		if kind == "" {
			kind = c.Type
		}
		t.Rows = append(t.Rows, []string{c.Name, kind, sheet.Str(c.Category), sheet.Str(c.Industry), sheet.Str(c.City),
			sheet.Str(c.Website), sheet.Str(c.LinkedinURL), sheet.Day(c.UpdatedAt)})
	}
	return sheet.Write(format, sheet.Dated("Companies", time.Now()), t)
}

// ExportCompanies handles GET /v1/companies/export.
func (h *HTTP) ExportCompanies(ctx context.Context, req api.ExportCompaniesRequestObject) (api.ExportCompaniesResponseObject, error) {
	q := req.Params
	var kind *string
	if q.Type != nil {
		k := string(*q.Type)
		kind = &k
	}
	file, err := h.svc.Export(ctx, authz.MustFromContext(ctx), ListFilter{
		Query: q.Q, Boolean: q.Boolean, Type: kind, Category: q.Category, LinkedinCompany: q.LinkedinCompany, Folder: q.Folder,
	}, string(q.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportCompanies200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportCompanies200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
