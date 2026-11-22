package contacts

// Exporting the contact list (what its filters find) to Excel or CSV.

import (
	"bytes"
	"context"
	"mime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var statusLabel = map[string]string{"active": "Active", "left": "Left the company"}

// roleText writes a role key from Settings for people: hr is HR, other keys
// (hiring_manager, or one the agency added) read as words.
func roleText(role *string) string {
	if role == nil {
		return ""
	}
	if *role == "hr" {
		return "HR"
	}
	words := strings.ReplaceAll(*role, "_", " ")
	return strings.ToUpper(words[:1]) + words[1:]
}

// Export returns the contacts the filter finds as a file.
func (s *Service) Export(ctx context.Context, p authz.Principal, f Filter, format string) (sheet.File, error) {
	f.Similar = false // likeness has one page: an export is the plain list
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
			Action: "contact.exported", EntityType: "contact", EntityID: p.UserID,
			Changes: map[string]any{"count": len(found), "format": format},
		})
	})
	if err != nil {
		return sheet.File{}, err
	}
	t := sheet.Table{Name: "Contacts", Columns: []sheet.Column{
		{Heading: "Name"}, {Heading: "Title"}, {Heading: "Company"}, {Heading: "Role"}, {Heading: "Email"},
		{Heading: "Phone"}, {Heading: "Mobile"}, {Heading: "Status"}, {Heading: "Updated"},
	}}
	for _, r := range found {
		status := statusLabel[r.Status]
		if status == "" {
			status = r.Status
		}
		t.Rows = append(t.Rows, []string{FullName(r.FirstName, r.LastName), sheet.Str(r.Title), r.CompanyName, roleText(r.Role),
			sheet.Str(r.Email), sheet.Str(r.Phone), sheet.Str(r.Mobile), status, sheet.Day(r.UpdatedAt)})
	}
	return sheet.Write(format, sheet.Dated("Contacts", time.Now()), t)
}

// ExportContacts handles GET /v1/contacts/export.
func (h *HTTP) ExportContacts(ctx context.Context, req api.ExportContactsRequestObject) (api.ExportContactsResponseObject, error) {
	q := req.Params
	file, err := h.svc.Export(ctx, authz.MustFromContext(ctx), Filter{
		Query: q.Q, Boolean: q.Boolean, CompanyID: q.CompanyId, Status: c.Convert[string](q.Status), Role: q.Role,
		RoleNotSet: q.RoleNotSet != nil && *q.RoleNotSet, Folder: q.Folder,
	}, string(q.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportContacts200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportContacts200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
