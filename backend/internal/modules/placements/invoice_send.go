package placements

// Sending an invoice: made up from the placement, its fee, its recipient and
// the agency's template; numbered; emailed as a PDF; the placement marked
// invoiced. Also the invoice to download, as sent or as a preview.

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// SendNow emails a placement's invoice at once.
func (s *Service) SendNow(ctx context.Context, p authz.Principal, id string) (View, error) {
	if !p.Can(api.PermissionPlacementsFeesRead) {
		return View{}, apperr.New(api.ErrorCodeFORBIDDEN, "You cannot send invoices: they show the fee.")
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
		inv, err := q.LockPlacementInvoicing(ctx, id)
		if database.IsNoRows(err) {
			return notAllowed("Choose who receives the invoice first.")
		}
		if err != nil {
			return err
		}
		switch {
		case row.InvoiceStatus != NotInvoiced || inv.SentAt != nil:
			return notAllowed("This placement is already invoiced.")
		case row.Status == Cancelled:
			return notAllowed("This placement was cancelled: there is nothing to invoice.")
		}
		if err := s.send(ctx, tx, p.TenantID, row, inv, false); err != nil {
			var reason *notReady
			if errors.As(err, &reason) {
				return notAllowed(reason.message)
			}
			return err
		}
		row, err = q.GetPlacement(ctx, id)
		if err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// notReady is why an invoice cannot be made yet (no fee, no recipient).
type notReady struct{ message string }

func (e *notReady) Error() string { return e.message }

// invoice is an invoice made up: its fields' values, who it is to and the document.
type invoice struct {
	values  map[string]string
	contact Contact
	app     Application
	pdf     []byte
	docx    []byte
	from    string
	number  string
	due     time.Time
}

// makeInvoice puts a placement's invoice together, numbered number and dated
// date (preview: the number shown as such). It needs the fee and a recipient.
func (s *Service) makeInvoice(ctx context.Context, tx pgx.Tx, tenantID string, row store.Placement, inv store.PlacementInvoice,
	number string, date time.Time, format string) (invoice, error) {
	q := store.New(tx)
	apps, err := s.applications.Briefs(ctx, tx, []string{row.ApplicationID})
	if err != nil {
		return invoice{}, err
	}
	app := OnSheet(apps[row.ApplicationID], detailsOf(row).Sheet)
	if inv.ContactID == nil {
		return invoice{}, &notReady{"Choose who receives the invoice (one of the client's contacts)."}
	}
	contacts, err := s.inv.Contacts.Contacts(ctx, tx, []string{*inv.ContactID})
	if err != nil {
		return invoice{}, err
	}
	contact, ok := contacts[*inv.ContactID]
	if !ok || contact.Email == nil {
		return invoice{}, &notReady{"The invoice recipient has no email address."}
	}
	moneyRows, err := q.PlacementMoney(ctx, []string{row.ID})
	if err != nil {
		return invoice{}, err
	}
	var money *Money
	if len(moneyRows) == 1 {
		if money, err = moneyOf(moneyRows[0]); err != nil {
			return invoice{}, err
		}
	}
	if money == nil || money.FeeAmount == nil || money.FeeCurrency == nil {
		return invoice{}, &notReady{"Record the placement's fee first."}
	}
	settings, err := s.settingsIn(ctx, tx, tenantID)
	if err != nil {
		return invoice{}, err
	}
	agency, err := s.inv.Tenants.TenantName(ctx, tx, tenantID)
	if err != nil {
		return invoice{}, err
	}
	// The first placement on a job with a retainer invoices what is left of the fee.
	fee, less := *money.FeeAmount, ""
	if rv, err := s.retainersOff(ctx, tx, []string{app.JobID}); err != nil {
		return invoice{}, err
	} else if r, ok := rv[row.ID]; ok {
		if balance, ok := r.Balance(fee, *money.FeeCurrency); ok {
			if balance == "0.00" {
				return invoice{}, &notReady{"The retainer covers the whole fee: nothing is left to invoice."}
			}
			fee, less = balance, "less the retainer of "+MoneyText(r.Currency, r.Total())
		}
	}
	totals, err := InvoiceAmounts(fee, settings.TaxPercent)
	if err != nil {
		return invoice{}, err
	}
	cur := *money.FeeCurrency
	due := DueOn(date, int(inv.PaymentTermsDays), inv.DueOn)
	first, _, _ := strings.Cut(strings.TrimSpace(contact.Name), " ")
	values := map[string]string{
		"agency.name": agency, "client.name": app.CompanyName, "contact.name": contact.Name, "contact.firstName": first,
		"candidate.name": app.CandidateName, "job.title": app.JobTitle, "placement.startDate": LongDate(row.StartDate),
		"placement.poNumber": deref(row.PoNumber),
		"invoice.number":     number, "invoice.date": LongDate(date), "invoice.dueDate": LongDate(due),
		"invoice.terms": strconv.Itoa(int(inv.PaymentTermsDays)), "invoice.subtotal": MoneyText(cur, totals.Subtotal),
		"invoice.tax": MoneyText(cur, totals.Tax), "invoice.total": MoneyText(cur, totals.Total),
		"fee.percent": "", "fee.basis": "", "salary.annual": "",
	}
	if money.FeePercent != nil && money.Salary != nil {
		annual := annualSalary(*money.Salary)
		values["fee.percent"] = trimZeros(*money.FeePercent) + "%"
		values["salary.annual"] = MoneyText(money.Salary.Currency, annual)
		values["fee.basis"] = values["fee.percent"] + " of the annual salary of " + values["salary.annual"]
	}
	if less != "" {
		values["fee.basis"] = strings.TrimPrefix(values["fee.basis"]+", "+less, ", ")
	}
	billTo := []string{app.CompanyName, contact.Name, *contact.Email}
	doc := InvoiceDoc(settings, values, billTo, totals.Tax != "0.00")
	out := invoice{values: values, contact: contact, app: app, from: agency, number: number, due: due}
	if format == "docx" {
		out.docx, err = doc.Docx()
	} else {
		out.pdf, err = doc.PDF()
	}
	return out, err
}

// send numbers, makes and emails a placement's invoice, and marks the
// placement invoiced. inv is locked.
func (s *Service) send(ctx context.Context, tx pgx.Tx, tenantID string, row store.Placement, inv store.PlacementInvoice, auto bool) error {
	q := store.New(tx)
	// Check it can be made before a number is taken.
	if err := s.ready(ctx, tx, row, inv); err != nil {
		return err
	}
	if err := s.ensureSettings(ctx, tx, tenantID); err != nil {
		return err
	}
	number, err := s.takeNumber(ctx, q)
	if err != nil {
		return err
	}
	today := s.localToday()
	made, err := s.makeInvoice(ctx, tx, tenantID, row, inv, number, today, "pdf")
	if err != nil {
		return err
	}
	settings, err := s.settingsIn(ctx, tx, tenantID)
	if err != nil {
		return err
	}
	if err := s.inv.Mailer.SendInvoice(ctx, tx, tenantID, InvoiceMail{
		To: *made.contact.Email, From: made.from, ApplicationID: row.ApplicationID,
		Subject: FillInvoice(settings.EmailSubject, made.values), Body: FillInvoice(settings.EmailBody, made.values),
		FileName: "Invoice " + number + ".pdf", FileKey: "invoice_" + row.ID + "_" + number, PDF: made.pdf,
	}); err != nil {
		return err
	}
	if err := q.MarkPlacementInvoiced(ctx, store.MarkPlacementInvoicedParams{ID: row.ID, InvoiceNumber: &number, InvoicedOn: &today}); err != nil {
		return err
	}
	if err := q.MarkInvoiceSent(ctx, store.MarkInvoiceSentParams{PlacementID: row.ID, DueOn: &made.due}); err != nil {
		return err
	}
	// The number and recipient, never the amounts.
	return audit.Record(ctx, tx, audit.Entry{Action: "placement.invoice_sent", EntityType: "placement", EntityID: row.ID, Changes: map[string]any{
		"invoiceNumber": number, "to": *made.contact.Email, "automatic": auto, "dueOn": made.due.Format(time.DateOnly),
	}})
}

// ready reports why a placement's invoice cannot be made yet (no
// recipient with an email address, no fee), without making it.
func (s *Service) ready(ctx context.Context, tx pgx.Tx, row store.Placement, inv store.PlacementInvoice) error {
	if inv.ContactID == nil {
		return &notReady{"Choose who receives the invoice (one of the client's contacts)."}
	}
	contacts, err := s.inv.Contacts.Contacts(ctx, tx, []string{*inv.ContactID})
	if err != nil {
		return err
	}
	if c, ok := contacts[*inv.ContactID]; !ok || c.Email == nil {
		return &notReady{"The invoice recipient has no email address."}
	}
	money, err := store.New(tx).PlacementMoney(ctx, []string{row.ID})
	if err != nil {
		return err
	}
	if len(money) == 0 || !money[0].FeeAmount.Valid || money[0].FeeCurrency == nil {
		return &notReady{"Record the placement's fee first."}
	}
	return nil
}

// takeNumber takes the next invoice number, passing over any already on an
// invoice (typed in by hand, or a counter set back).
func (s *Service) takeNumber(ctx context.Context, q *store.Queries) (string, error) {
	for range 1000 {
		taken, err := q.TakeInvoiceNumber(ctx)
		if err != nil {
			return "", err
		}
		number := InvoiceNumber(taken.NumberPrefix, int(taken.Number))
		used, err := q.InvoiceNumberUsed(ctx, &number)
		if err != nil {
			return "", err
		}
		if !used {
			return number, nil
		}
	}
	return "", errors.New("no free invoice number in the next 1000")
}

// InvoiceFile is an invoice to download.
type InvoiceFile struct {
	FileName string
	Data     []byte
}

// File returns a placement's invoice as a PDF or Word file: as sent (with
// its number) once invoiced, a preview before.
func (s *Service) File(ctx context.Context, p authz.Principal, id, format string) (InvoiceFile, error) {
	if !p.Can(api.PermissionPlacementsFeesRead) {
		return InvoiceFile{}, apperr.New(api.ErrorCodeFORBIDDEN, "You cannot see invoices: they show the fee.")
	}
	var out InvoiceFile
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetPlacement(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("placement")
		}
		if err != nil {
			return err
		}
		invs, err := q.ListPlacementInvoicing(ctx, []string{id}) // read only: no lock
		if err != nil {
			return err
		}
		if len(invs) == 0 {
			return notAllowed("Choose who receives the invoice first.")
		}
		inv := invs[0]
		number, date := "(numbered when sent)", row.StartDate
		if row.InvoiceNumber != nil {
			number = *row.InvoiceNumber
		}
		if row.InvoicedOn != nil {
			date = *row.InvoicedOn
		}
		made, err := s.makeInvoice(ctx, tx, p.TenantID, row, inv, number, date, format)
		var reason *notReady
		if errors.As(err, &reason) {
			return notAllowed(reason.message)
		}
		if err != nil {
			return err
		}
		name := "Invoice " + made.app.CandidateName
		if row.InvoiceNumber != nil {
			name = "Invoice " + *row.InvoiceNumber
		}
		out = InvoiceFile{FileName: name + "." + format, Data: made.pdf}
		if format == "docx" {
			out.Data = made.docx
		}
		return nil
	})
	return out, err
}
