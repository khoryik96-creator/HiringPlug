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
	d, errs, _ := Normalize(d, s.today())
	m, moneyErrs := normalizeMoney(money)
	if err := append(errs, moneyErrs...).Err(); err != nil {
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
		if err := s.checkRecruiter(ctx, tx, d.RecruiterID); err != nil {
			return err
		}
		q := store.New(tx)
		row, err := q.InsertPlacement(ctx, store.InsertPlacementParams{
			ApplicationID: appID, RecruiterID: d.RecruiterID, StartDate: d.StartDate, GuaranteeDays: int32(d.GuaranteeDays), //nolint:gosec // 0..365
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
		changes := map[string]any{"applicationId": appID, "startDate": d.StartDate.Format(time.DateOnly), "guaranteeDays": d.GuaranteeDays, "recruiterId": d.RecruiterID}
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
			Status: after.Status, LeftOn: after.LeftOn, StatusNote: after.StatusNote,
			InvoiceStatus: after.InvoiceStatus, InvoiceNumber: after.InvoiceNumber, InvoicedOn: after.InvoicedOn, PaidOn: after.PaidOn,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("placement")
		}
		if err != nil {
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

// List returns placements, newest start first, and (with placements.fees:read)
// the fee totals of those that stand.
func (s *Service) List(ctx context.Context, p authz.Principal, f Filter) ([]View, []FeeTotal, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListPlacements(ctx, store.ListPlacementsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, p, rows)
		return err
	})
	if err != nil || !p.Can(api.PermissionPlacementsFeesRead) {
		return out, nil, err
	}
	var lines []FeeLine
	for _, v := range out {
		if v.Money != nil && v.Money.FeeAmount != nil {
			lines = append(lines, FeeLine{Status: v.Details.Status, InvoiceStatus: v.Details.InvoiceStatus, Amount: *v.Money.FeeAmount, Currency: *v.Money.FeeCurrency})
		}
	}
	return out, Totals(lines), nil
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
		Status: r.Status, LeftOn: r.LeftOn, StatusNote: r.StatusNote,
		InvoiceStatus: r.InvoiceStatus, InvoiceNumber: r.InvoiceNumber, InvoicedOn: r.InvoicedOn, PaidOn: r.PaidOn,
	}
}

func auditFields(d Details) map[string]any {
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format(time.DateOnly)
	}
	return map[string]any{
		"recruiterId": d.RecruiterID, "startDate": d.StartDate.Format(time.DateOnly), "guaranteeDays": d.GuaranteeDays,
		"status": d.Status, "leftOn": day(d.LeftOn), "statusNote": d.StatusNote,
		"invoiceStatus": d.InvoiceStatus, "invoiceNumber": d.InvoiceNumber, "invoicedOn": day(d.InvoicedOn), "paidOn": day(d.PaidOn),
	}
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
	out := make([]View, len(rows))
	for i, r := range rows {
		v := View{
			ID: r.ID, Version: int(r.Version), ApplicationID: r.ApplicationID, App: apps[r.ApplicationID], Details: detailsOf(r),
			Money: money[r.ID], MoneyHidden: !seeMoney, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
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
