package placements

// Exporting placements to Excel or CSV. The salary and fee are in it only
// for roles that may see fees (placements.fees:read), and never in the
// audit entry.

import (
	"bytes"
	"context"
	"mime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var (
	placementStatusLabel = map[string]string{"active": "Active", "fell_through": "Fell through", "cancelled": "Cancelled", "backed_out": "Backed out"}
	invoiceStatusLabel   = map[string]string{"not_invoiced": "Not invoiced", "invoiced": "Invoiced", "paid": "Paid"}
)

// Export returns the placements the filter finds as a file.
func (s *Service) Export(ctx context.Context, p authz.Principal, f Filter, format string) (sheet.File, error) {
	var views []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ExportPlacements(ctx, store.ExportPlacementsParams{
			Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		if err != nil {
			return err
		}
		if len(rows) > sheet.MaxRows {
			return apperr.ExportTooLarge(sheet.MaxRows)
		}
		if views, err = s.views(ctx, tx, p, rows); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "placement.exported", EntityType: "placement", EntityID: p.UserID,
			Changes: map[string]any{"count": len(views), "format": format, "fees": p.Can(api.PermissionPlacementsFeesRead)},
		})
	})
	if err != nil {
		return sheet.File{}, err
	}
	return sheet.Write(format, sheet.Dated("Placements", time.Now()), PlacementTable(views, p.Can(api.PermissionPlacementsFeesRead)))
}

// PlacementTable is the export's sheet: the salary and fee columns only when shown.
func PlacementTable(views []View, fees bool) sheet.Table {
	cols := []sheet.Column{
		{Heading: "Candidate"}, {Heading: "Job"}, {Heading: "Client"}, {Heading: "Recruiter"}, {Heading: "Start date"},
		{Heading: "Guarantee days", Kind: sheet.Number}, {Heading: "Guarantee ends"}, {Heading: "Status"}, {Heading: "Invoice"},
		{Heading: "Invoice number"}, {Heading: "Invoiced on"}, {Heading: "Paid on"},
	}
	if fees {
		cols = append(cols, sheet.Column{Heading: "Salary", Kind: sheet.Number}, sheet.Column{Heading: "Salary currency"}, sheet.Column{Heading: "Salary per"},
			sheet.Column{Heading: "Fee %", Kind: sheet.Number}, sheet.Column{Heading: "Fee", Kind: sheet.Number}, sheet.Column{Heading: "Fee currency"})
	}
	t := sheet.Table{Name: "Placements", Columns: cols, Rows: make([][]string, 0, len(views))}
	for _, v := range views {
		d := v.Details
		row := []string{
			v.App.CandidateName, v.App.JobTitle, v.App.CompanyName, sheet.Str(v.RecruiterName), sheet.Day(d.StartDate),
			strconv.Itoa(d.GuaranteeDays), sheet.Day(d.GuaranteeEnds()), labelOf(placementStatusLabel, d.Status),
			labelOf(invoiceStatusLabel, d.InvoiceStatus), sheet.Str(d.InvoiceNumber), exportDay(d.InvoicedOn), exportDay(d.PaidOn),
		}
		if fees {
			money := make([]string, 6)
			if m := v.Money; m != nil {
				if m.Salary != nil {
					money[0], money[1], money[2] = m.Salary.Amount, m.Salary.Currency, m.Salary.Period
				}
				money[3], money[4], money[5] = sheet.Str(m.FeePercent), sheet.Str(m.FeeAmount), sheet.Str(m.FeeCurrency)
			}
			row = append(row, money...)
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

func labelOf(labels map[string]string, v string) string {
	if l, ok := labels[v]; ok {
		return l
	}
	return v
}

func exportDay(t *time.Time) string {
	if t == nil {
		return ""
	}
	return sheet.Day(*t)
}

// ExportPlacements handles GET /v1/placements/export.
func (h *HTTP) ExportPlacements(ctx context.Context, req api.ExportPlacementsRequestObject) (api.ExportPlacementsResponseObject, error) {
	q := req.Params
	file, err := h.svc.Export(ctx, authz.MustFromContext(ctx), Filter{
		Status: c.Convert[string](q.Status), InvoiceStatus: c.Convert[string](q.InvoiceStatus), StartFrom: dayPtr(q.StartFrom), StartTo: dayPtr(q.StartTo),
	}, string(q.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportPlacements200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportPlacements200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
