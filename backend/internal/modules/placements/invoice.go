package placements

// Invoicing a placement: what it starts with (the client's terms), who
// receives the invoice and when payment is due, and the invoice itself:
// numbered from the agency's template, laid out as a PDF and emailed on the
// start date (or at once, on request); the placement is then invoiced.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Defaults are what a new placement starts with.
type Defaults struct {
	CompanyID, CompanyName          string
	Terms                           ClientTerms
	FeePercent                      *string // the rate for any salary; nil when it depends on the salary
	PaymentTermsDays, GuaranteeDays int
	ContactID                       *string // the job's hiring contact, when they have an email address
}

// InvoicingView is a placement's invoicing as shown.
type InvoicingView struct {
	ContactID, ContactName, ContactEmail *string
	PaymentTermsDays                     int
	DueOn                                time.Time
	DueOnSet, AutoSend                   bool
	SendsAt, SentAt                      *time.Time
	SendError                            *string
}

// InvoicingInput is a person's choice of recipient, terms, due date and emailing.
type InvoicingInput struct {
	ContactID        *string
	PaymentTermsDays int
	DueOn            *time.Time // nil: the invoice date plus the terms
	AutoSend         bool
}

// Defaults returns what a placement for an application starts with.
func (s *Service) Defaults(ctx context.Context, p authz.Principal, appID string) (Defaults, error) {
	var out Defaults
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications.Briefs(ctx, tx, []string{appID})
		if err != nil {
			return err
		}
		app, ok := apps[appID]
		if !ok {
			return apperr.NotFound("application")
		}
		terms, err := s.inv.Terms.Terms(ctx, tx, app.CompanyID)
		if err != nil {
			return err
		}
		out = Defaults{CompanyID: app.CompanyID, CompanyName: app.CompanyName, Terms: terms,
			PaymentTermsDays: DefaultPaymentTermsDays, GuaranteeDays: DefaultGuaranteeDays}
		if rate, ok := terms.For(nil); ok {
			out.FeePercent = rate.FeePercent
			if rate.PaymentTermsDays != nil {
				out.PaymentTermsDays = *rate.PaymentTermsDays
			}
			if rate.GuaranteeDays != nil {
				out.GuaranteeDays = *rate.GuaranteeDays
			}
		}
		out.ContactID, err = s.hiringContact(ctx, tx, app)
		return err
	})
	return out, err
}

// hiringContact is the job's hiring contact, when they have an email address.
func (s *Service) hiringContact(ctx context.Context, tx pgx.Tx, app Application) (*string, error) {
	if app.ContactID == nil {
		return nil, nil //nolint:nilnil // no hiring contact is not an error
	}
	contacts, err := s.inv.Contacts.Contacts(ctx, tx, []string{*app.ContactID})
	if err != nil {
		return nil, err
	}
	if c, ok := contacts[*app.ContactID]; ok && c.Email != nil {
		return app.ContactID, nil
	}
	return nil, nil //nolint:nilnil // a contact without an email address is no recipient
}

// startInvoicing gives a new placement its client's payment terms (the rate
// for its salary) and the job's hiring contact as the recipient (not emailed
// until someone says so).
func (s *Service) startInvoicing(ctx context.Context, tx pgx.Tx, placementID string, app Application, salary *Salary) error {
	terms, err := s.inv.Terms.Terms(ctx, tx, app.CompanyID)
	if err != nil {
		return err
	}
	days := DefaultPaymentTermsDays
	if rate, ok := terms.For(salary); ok && rate.PaymentTermsDays != nil {
		days = *rate.PaymentTermsDays
	}
	contact, err := s.hiringContact(ctx, tx, app)
	if err != nil {
		return err
	}
	return store.New(tx).PutPlacementInvoicing(ctx, store.PutPlacementInvoicingParams{
		PlacementID: placementID, ContactID: contact, PaymentTermsDays: int32(days), //nolint:gosec // 0..365
	})
}

// PutInvoicing sets who receives a placement's invoice, the terms and due
// date, and whether it is emailed on the start date.
func (s *Service) PutInvoicing(ctx context.Context, p authz.Principal, id string, in InvoicingInput) (View, error) {
	if in.PaymentTermsDays < 0 || in.PaymentTermsDays > 365 {
		return View{}, apperr.Validation(apperr.FieldErrors{{Field: "paymentTermsDays", Message: "between 0 and 365 days"}})
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetPlacement(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("placement")
		}
		if err != nil {
			return err
		}
		apps, err := s.applications.Briefs(ctx, tx, []string{row.ApplicationID})
		if err != nil {
			return err
		}
		invoiceDate := row.StartDate
		if row.InvoicedOn != nil {
			invoiceDate = *row.InvoicedOn
		}
		if in.DueOn != nil && in.DueOn.Before(invoiceDate) {
			return apperr.Validation(apperr.FieldErrors{{Field: "dueOn", Message: "must be on or after the invoice date (" + LongDate(invoiceDate) + ")"}})
		}
		if in.ContactID != nil {
			contacts, err := s.inv.Contacts.Contacts(ctx, tx, []string{*in.ContactID})
			if err != nil {
				return err
			}
			c, ok := contacts[*in.ContactID]
			switch {
			case !ok || c.CompanyID != apps[row.ApplicationID].CompanyID:
				return apperr.Validation(apperr.FieldErrors{{Field: "contactId", Message: "choose one of the client's contacts"}})
			case c.Email == nil && in.AutoSend:
				return apperr.Validation(apperr.FieldErrors{{Field: "contactId", Message: "has no email address to send the invoice to"}})
			}
		} else if in.AutoSend {
			return apperr.Validation(apperr.FieldErrors{{Field: "contactId", Message: "choose who receives the invoice"}})
		}
		if err := q.PutPlacementInvoicing(ctx, store.PutPlacementInvoicingParams{
			PlacementID: id, ContactID: in.ContactID, PaymentTermsDays: int32(in.PaymentTermsDays), //nolint:gosec // 0..365
			DueOn: in.DueOn, AutoSend: in.AutoSend,
		}); err != nil {
			return err
		}
		if in.AutoSend && row.Status == Active && row.InvoiceStatus == NotInvoiced {
			if err := s.schedule(ctx, tx, p.TenantID, id, row.StartDate); err != nil {
				return err
			}
		}
		due := any(nil)
		if in.DueOn != nil {
			due = in.DueOn.Format(time.DateOnly)
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "placement.invoicing_set", EntityType: "placement", EntityID: id, Changes: map[string]any{
			"contactId": in.ContactID, "paymentTermsDays": in.PaymentTermsDays, "dueOn": due, "autoSend": in.AutoSend,
		}}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// schedule queues the emailing of a placement's invoice on its start date,
// unless it is already queued for that time. A job for an earlier time finds
// the time moved and does nothing (or queues it again for the new time).
func (s *Service) schedule(ctx context.Context, tx pgx.Tx, tenantID, placementID string, start time.Time) error {
	settings, err := s.settingsIn(ctx, tx, tenantID)
	if err != nil {
		return err
	}
	// Queued once per send time (the start date at the hour): the job clears
	// it when it runs, so anything later queues it afresh.
	when := SendTime(start, settings.SendHour, s.inv.TimeZone)
	n, err := store.New(tx).MarkInvoiceScheduled(ctx, store.MarkInvoiceScheduledParams{PlacementID: placementID, ScheduledFor: &when})
	if err != nil || n == 0 {
		return err
	}
	at := SendsAt(start, settings.SendHour, s.inv.TimeZone, s.clock()) // at once when that time is past
	_, err = jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindPlacementInvoice, api.PlacementInvoiceJob{PlacementId: placementID}, at)
	return err
}

// rescheduleAll queues again every invoice still to be emailed (the hour changed).
func (s *Service) rescheduleAll(ctx context.Context, tx pgx.Tx, tenantID string) error {
	rows, err := store.New(tx).ListAwaitingInvoices(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.schedule(ctx, tx, tenantID, r.ID, r.StartDate); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// localToday is today's date in the agency's time zone.
func (s *Service) localToday() time.Time {
	y, m, d := s.clock().In(s.inv.TimeZone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// invoiceJob handles placement.invoice: on the start date, the invoice is
// emailed if it is still to be. A start date moved later queues it again.
func (s *Service) invoiceJob(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.PlacementInvoiceJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	inv, err := q.LockPlacementInvoicing(ctx, payload.PlacementId)
	if database.IsNoRows(err) {
		return nil // the placement is gone
	}
	if err != nil {
		return err
	}
	if err := q.ClearInvoiceSchedule(ctx, payload.PlacementId); err != nil {
		return err
	}
	row, err := q.GetPlacement(ctx, payload.PlacementId)
	if err != nil {
		return err
	}
	if !inv.AutoSend || inv.SentAt != nil || row.Status != Active || row.InvoiceStatus != NotInvoiced {
		return nil
	}
	settings, err := s.settingsIn(ctx, tx, job.TenantID)
	if err != nil {
		return err
	}
	if s.clock().Before(SendTime(row.StartDate, settings.SendHour, s.inv.TimeZone)) {
		return s.schedule(ctx, tx, job.TenantID, row.ID, row.StartDate) // the start date or hour moved later
	}
	err = s.send(ctx, tx, job.TenantID, row, inv, true)
	var reason *notReady
	if errors.As(err, &reason) {
		logx.From(ctx).Info("invoice not emailed", "placement_id", row.ID, "reason", reason.message)
		msg := reason.message
		return q.SetInvoiceError(ctx, store.SetInvoiceErrorParams{PlacementID: row.ID, SendError: &msg})
	}
	return err
}

// reschedule queues the invoice email again after the start date moved.
func (s *Service) reschedule(ctx context.Context, tx pgx.Tx, tenantID string, row store.Placement) error {
	inv, err := store.New(tx).ListPlacementInvoicing(ctx, []string{row.ID})
	if err != nil || len(inv) == 0 {
		return err
	}
	if inv[0].AutoSend && inv[0].SentAt == nil && row.Status == Active && row.InvoiceStatus == NotInvoiced {
		return s.schedule(ctx, tx, tenantID, row.ID, row.StartDate)
	}
	return nil
}

// invoicingViews are the placements' invoicing as shown, by placement id.
func (s *Service) invoicingViews(ctx context.Context, tx pgx.Tx, tenantID string, rows []store.Placement) (map[string]InvoicingView, error) {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	invRows, err := store.New(tx).ListPlacementInvoicing(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.PlacementInvoice{}
	var contactIDs []string
	for _, r := range invRows {
		byID[r.PlacementID] = r
		if r.ContactID != nil {
			contactIDs = append(contactIDs, *r.ContactID)
		}
	}
	contacts := map[string]Contact{}
	if len(contactIDs) > 0 {
		if contacts, err = s.inv.Contacts.Contacts(ctx, tx, contactIDs); err != nil {
			return nil, err
		}
	}
	hour := -1 // read once, when some invoice is to go out
	out := make(map[string]InvoicingView, len(rows))
	for _, r := range rows {
		inv, ok := byID[r.ID]
		if !ok {
			inv = store.PlacementInvoice{PaymentTermsDays: DefaultPaymentTermsDays}
		}
		invoiceDate := r.StartDate
		if r.InvoicedOn != nil {
			invoiceDate = *r.InvoicedOn
		}
		v := InvoicingView{
			ContactID: inv.ContactID, PaymentTermsDays: int(inv.PaymentTermsDays), DueOn: DueOn(invoiceDate, int(inv.PaymentTermsDays), inv.DueOn),
			DueOnSet: inv.DueOn != nil, AutoSend: inv.AutoSend, SentAt: inv.SentAt, SendError: inv.SendError,
		}
		if inv.ContactID != nil {
			if c, ok := contacts[*inv.ContactID]; ok {
				v.ContactName, v.ContactEmail = &c.Name, c.Email
			}
		}
		if inv.AutoSend && inv.SentAt == nil && inv.SendError == nil && r.Status == Active && r.InvoiceStatus == NotInvoiced && v.ContactEmail != nil {
			if hour < 0 {
				settings, err := s.settingsIn(ctx, tx, tenantID)
				if err != nil {
					return nil, err
				}
				hour = settings.SendHour
			}
			at := SendsAt(r.StartDate, hour, s.inv.TimeZone, s.clock())
			v.SendsAt = &at
		}
		out[r.ID] = v
	}
	return out, nil
}
