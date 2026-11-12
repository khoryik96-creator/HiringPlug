package placements

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the placement use cases.
type Service struct {
	pool         *pgxpool.Pool
	applications Applications
	users        Users
	inv          Invoicing
	now          func() time.Time // tests may fix "today"
}

// View is a placement as a user may see it.
type View struct {
	ID            string
	Version       int
	ApplicationID string
	App           Application
	Details       Details
	RecruiterName *string
	Money         *Money // nil: none recorded, or hidden (MoneyHidden)
	MoneyHidden   bool
	Invoicing     InvoicingView
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const onePerApplication = "placement_tenant_id_application_id_key"

func (s *Service) today() time.Time {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	y, m, d := now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func notAllowed(message string) error { return apperr.New(api.ErrorCodePLACEMENTNOTALLOWED, message) }

func mayChangeMoney(p authz.Principal) error {
	if !p.Can(api.PermissionPlacementsFeesUpdate) {
		return apperr.New(api.ErrorCodeFORBIDDEN, "You cannot set the salary or fee of a placement.")
	}
	return nil
}

// Create records a placement for an application and moves it to placed.
func (s *Service) Create(ctx context.Context, p authz.Principal, appID string, d Details, money *Money) (View, error) {
	if money != nil {
		if err := mayChangeMoney(p); err != nil {
			return View{}, err
		}
	}
	if d.RecruiterID == nil {
		d.RecruiterID = &p.UserID
	}
	d.Status, d.InvoiceStatus = Active, NotInvoiced
	m, moneyErrs := normalizeMoney(money)
	if err := moneyErrs.Err(); err != nil {
		return View{}, err
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications.Briefs(ctx, tx, []string{appID})
		if err != nil {
			return err
		}
		app, ok := apps[appID]
		if !ok {
			return apperr.NotFound("application")
		}
		if app.Stage == "rejected" || app.Stage == "withdrawn" {
			return notAllowed("Reopen the application (move it to an open stage) before recording a placement.")
		}
		d.Sheet = d.WithJob(app) // the sheet starts from the job
		var errs apperr.FieldErrors
		if d, errs, _ = Normalize(d, s.today()); errs.Err() != nil {
			return errs.Err()
		}
		if err := s.checkRecruiter(ctx, tx, d.RecruiterID); err != nil {
			return err
		}
		q := store.New(tx)
		row, err := q.InsertPlacement(ctx, store.InsertPlacementParams{
			ApplicationID: appID, RecruiterID: d.RecruiterID, StartDate: d.StartDate, GuaranteeDays: int32(d.GuaranteeDays), //nolint:gosec // 0..365
			JobTitle: d.JobTitle, CompanyName: d.CompanyName, JobType: d.JobType, PoNumber: d.PONumber, EndDate: d.EndDate,
		})
		if database.IsUniqueViolation(err, onePerApplication) {
			return notAllowed("This application already has a placement: change that one.")
		}
		if err != nil {
			return err
		}
		if err := s.applications.Place(ctx, tx, appID); err != nil {
			return err
		}
		if err := putMoney(ctx, q, row.ID, m); err != nil {
			return err
		}
		var salary *Salary
		if m != nil {
			salary = m.Salary
		}
		if err := s.startInvoicing(ctx, tx, row.ID, OnSheet(app, d.Sheet), salary); err != nil {
			return err
		}
		changes := sheetAudit(d.Sheet)
		changes["applicationId"], changes["startDate"], changes["guaranteeDays"], changes["recruiterId"] = appID, d.StartDate.Format(time.DateOnly), d.GuaranteeDays, d.RecruiterID
		if m != nil {
			changes["money"] = audit.Redacted
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "placement.recorded", EntityType: "placement", EntityID: row.ID, Changes: changes}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// Update changes a placement, if nobody changed it since version. money nil
// leaves the salary and fee as they are; a set change replaces them (a nil
// value clears them).
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, version int, patch Patch, money apiconv.Change[*Money]) (View, error) {
	if money.Set {
		if err := mayChangeMoney(p); err != nil {
			return View{}, err
		}
	}
	m, moneyErrs := normalizeMoney(money.Value)
	if err := moneyErrs.Err(); err != nil {
		return View{}, err
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
		if int(row.Version) != version {
			return apperr.StaleVersion("placement").WithDetail("currentVersion", row.Version)
		}
		before := detailsOf(row)
		after, errs, problem := Normalize(patch.Apply(before), s.today())
		if err := errs.Err(); err != nil {
			return err
		}
		if problem != "" {
			return notAllowed(problem)
		}
		if patch.RecruiterID.Set {
			if err := s.checkRecruiter(ctx, tx, after.RecruiterID); err != nil {
				return err
			}
		}
		updated, err := q.UpdatePlacement(ctx, store.UpdatePlacementParams{
			ID: id, ExpectedVersion: int32(version), //nolint:gosec // versions are small
			RecruiterID: after.RecruiterID, StartDate: after.StartDate, GuaranteeDays: int32(after.GuaranteeDays), //nolint:gosec // 0..365
			Status: after.Status, LeftOn: after.LeftOn, StatusNote: after.StatusNote, Replacement: after.Replacement,
			InvoiceStatus: after.InvoiceStatus, InvoiceNumber: after.InvoiceNumber, InvoicedOn: after.InvoicedOn, PaidOn: after.PaidOn,
			JobTitle: after.JobTitle, CompanyName: after.CompanyName, JobType: after.JobType, PoNumber: after.PONumber, EndDate: after.EndDate,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("placement")
		}
		if err != nil {
			return err
		}
		// A moved start date, a fee now recorded or a placement active again:
		// the invoice email is queued (again) for the start date.
		if err := s.reschedule(ctx, tx, p.TenantID, updated); err != nil {
			return err
		}
		changes := audit.Diff(auditFields(before), auditFields(after))
		if money.Set {
			if err := putMoney(ctx, q, id, m); err != nil {
				return err
			}
			changes["money"] = audit.Redacted // never the amounts
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "placement.updated", EntityType: "placement", EntityID: id, Changes: changes}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, updated)
		return err
	})
	return out, err
}

// Filter narrows the list of placements.
type Filter struct {
	ApplicationID, Status, InvoiceStatus *string
	StartFrom, StartTo                   *time.Time
}

// List returns placements, newest start first (at most 500), and (with
// placements.fees:read) the fee totals of every matching one that stands.
func (s *Service) List(ctx context.Context, p authz.Principal, f Filter) ([]View, []FeeTotal, error) {
	var out []View
	var totals []FeeTotal
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListPlacements(ctx, store.ListPlacementsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		if err != nil {
			return err
		}
		if out, err = s.views(ctx, tx, p, rows); err != nil || !p.Can(api.PermissionPlacementsFeesRead) {
			return err
		}
		sums, err := q.PlacementFeeTotals(ctx, store.PlacementFeeTotalsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		totals = feeTotals(len(sums), func(i int) (string, string, string) { return sums[i].Currency, sums[i].InvoiceStatus, sums[i].Total })
		return err
	})
	return out, totals, err
}

// Billing is what a client was billed: this year (in the agency's time
// zone) and all time, per currency.
type Billing struct {
	Year              int
	ThisYear, AllTime []FeeTotal
}

// CompanyBilling adds up the fees of the placements on a company's jobs.
func (s *Service) CompanyBilling(ctx context.Context, p authz.Principal, companyID string, now time.Time) (Billing, error) {
	year := now.In(s.inv.TimeZone).Year()
	out := Billing{Year: year, ThisYear: []FeeTotal{}, AllTime: []FeeTotal{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ids, err := s.applications.PlacedOfCompany(ctx, tx, companyID)
		if err != nil || len(ids) == 0 {
			return err
		}
		q := store.New(tx)
		from, to := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
		for _, period := range []struct {
			into     *[]FeeTotal
			from, to *time.Time
		}{{&out.ThisYear, &from, &to}, {&out.AllTime, nil, nil}} {
			sums, err := q.ApplicationsFeeTotals(ctx, store.ApplicationsFeeTotalsParams{ApplicationIds: ids, StartFrom: period.from, StartTo: period.to})
			if err != nil {
				return err
			}
			*period.into = feeTotals(len(sums), func(i int) (string, string, string) { return sums[i].Currency, sums[i].InvoiceStatus, sums[i].Total })
		}
		return nil
	})
	return out, err
}

// feeTotals turns the database's sums (currency, invoice status, total) into
// totals per currency.
func feeTotals(n int, row func(i int) (currency, invoiceStatus, total string)) []FeeTotal {
	lines := make([]FeeLine, n)
	for i := range lines {
		currency, invoiceStatus, total := row(i)
		lines[i] = FeeLine{Status: Active, InvoiceStatus: invoiceStatus, Amount: total, Currency: currency}
	}
	return Totals(lines)
}

func (s *Service) checkRecruiter(ctx context.Context, tx pgx.Tx, id *string) error {
	if id == nil {
		return nil
	}
	active, err := s.users.ActiveUsers(ctx, tx, []string{*id})
	if err != nil {
		return err
	}
	if _, ok := active[*id]; !ok {
		return apperr.Validation(apperr.FieldErrors{{Field: "recruiterId", Message: "is not an active user"}})
	}
	return nil
}

func normalizeMoney(m *Money) (*Money, apperr.FieldErrors) {
	if m == nil || m.Empty() {
		return nil, nil
	}
	out, errs := NormalizeMoney(*m)
	return &out, errs
}

// putMoney replaces a placement's salary and fee (nil clears them).
func putMoney(ctx context.Context, q *store.Queries, id string, m *Money) error {
	if m == nil {
		return q.DeletePlacementMoney(ctx, id)
	}
	params := store.PutPlacementMoneyParams{PlacementID: id, FeePercent: m.FeePercent, FeeAmount: m.FeeAmount, FeeCurrency: m.FeeCurrency}
	if m.Salary != nil {
		params.SalaryAmount, params.SalaryCurrency, params.SalaryPeriod = &m.Salary.Amount, &m.Salary.Currency, &m.Salary.Period
	}
	return q.PutPlacementMoney(ctx, params)
}

func detailsOf(r store.Placement) Details {
	return Details{
		RecruiterID: r.RecruiterID, StartDate: r.StartDate, GuaranteeDays: int(r.GuaranteeDays),
		Status: r.Status, LeftOn: r.LeftOn, StatusNote: r.StatusNote, Replacement: r.Replacement,
		InvoiceStatus: r.InvoiceStatus, InvoiceNumber: r.InvoiceNumber, InvoicedOn: r.InvoicedOn, PaidOn: r.PaidOn,
		Sheet: Sheet{JobTitle: r.JobTitle, CompanyName: r.CompanyName, JobType: r.JobType, PONumber: r.PoNumber, EndDate: r.EndDate},
	}
}

func auditFields(d Details) map[string]any {
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format(time.DateOnly)
	}
	out := sheetAudit(d.Sheet)
	for k, v := range map[string]any{
		"recruiterId": d.RecruiterID, "startDate": d.StartDate.Format(time.DateOnly), "guaranteeDays": d.GuaranteeDays,
		"status": d.Status, "leftOn": day(d.LeftOn), "statusNote": d.StatusNote, "replacement": d.Replacement,
		"invoiceStatus": d.InvoiceStatus, "invoiceNumber": d.InvoiceNumber, "invoicedOn": day(d.InvoicedOn), "paidOn": day(d.PaidOn),
	} {
		out[k] = v
	}
	return out
}

func (s *Service) one(ctx context.Context, tx pgx.Tx, p authz.Principal, row store.Placement) (View, error) {
	views, err := s.views(ctx, tx, p, []store.Placement{row})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// views adds the application, the recruiter's name and (when the user may
// see them) the salary and fee.
func (s *Service) views(ctx context.Context, tx pgx.Tx, p authz.Principal, rows []store.Placement) ([]View, error) {
	appIDs, userIDs, ids := make([]string, len(rows)), []string{}, make([]string, len(rows))
	for i, r := range rows {
		appIDs[i], ids[i] = r.ApplicationID, r.ID
		if r.RecruiterID != nil {
			userIDs = append(userIDs, *r.RecruiterID)
		}
	}
	apps, err := s.applications.Briefs(ctx, tx, appIDs)
	if err != nil {
		return nil, err
	}
	names, err := s.users.UserNames(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	seeMoney := p.Can(api.PermissionPlacementsFeesRead)
	money := map[string]*Money{}
	if seeMoney {
		moneyRows, err := store.New(tx).PlacementMoney(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range moneyRows {
			m, err := moneyOf(r)
			if err != nil {
				return nil, err
			}
			money[r.PlacementID] = m
		}
	}
	invoicing, err := s.invoicingViews(ctx, tx, p.TenantID, rows)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		v := View{
			ID: r.ID, Version: int(r.Version), ApplicationID: r.ApplicationID, App: OnSheet(apps[r.ApplicationID], detailsOf(r).Sheet), Details: detailsOf(r),
			Money: money[r.ID], MoneyHidden: !seeMoney, Invoicing: invoicing[r.ID], CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		if r.RecruiterID != nil {
			if n, ok := names[*r.RecruiterID]; ok {
				v.RecruiterName = &n
			}
		}
		out[i] = v
	}
	return out, nil
}

func moneyOf(r store.PlacementMoney) (*Money, error) {
	salary, err := database.DecimalString(r.SalaryAmount)
	if err != nil {
		return nil, err
	}
	pct, err := database.DecimalString(r.FeePercent)
	if err != nil {
		return nil, err
	}
	fee, err := database.DecimalString(r.FeeAmount)
	if err != nil {
		return nil, err
	}
	m := &Money{FeePercent: pct, FeeAmount: fee, FeeCurrency: r.FeeCurrency}
	if salary != nil && r.SalaryCurrency != nil && r.SalaryPeriod != nil {
		m.Salary = &Salary{Amount: *salary, Currency: *r.SalaryCurrency, Period: *r.SalaryPeriod}
	}
	return m, nil
}
