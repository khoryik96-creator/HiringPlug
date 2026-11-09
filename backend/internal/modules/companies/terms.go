package companies

// A client's terms of business: the fee (a percentage of the yearly salary),
// payment terms and guarantee they signed, which fill in a new placement.

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Terms are a client's terms of business; nil fields are not set.
type Terms struct {
	FeePercent                      *string // e.g. "18"
	PaymentTermsDays, GuaranteeDays *int
	SignedOn                        *time.Time
	Notes                           *string
	Version                         int // 0 before the first save
	UpdatedAt                       *time.Time
}

// NormalizeTerms checks terms a person saves.
func NormalizeTerms(t Terms) (Terms, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if t.FeePercent != nil {
		pct, ok := new(big.Rat).SetString(*t.FeePercent)
		if !ok || pct.Sign() <= 0 || pct.Cmp(big.NewRat(100, 1)) > 0 {
			errs.Add("feePercent", "a percentage above 0, at most 100")
		}
	}
	days := func(field string, v *int) {
		if v != nil && (*v < 0 || *v > 365) {
			errs.Add(field, "between 0 and 365 days")
		}
	}
	days("paymentTermsDays", t.PaymentTermsDays)
	days("guaranteeDays", t.GuaranteeDays)
	if t.Notes != nil {
		n := strings.TrimSpace(*t.Notes)
		t.Notes = &n
		if n == "" {
			t.Notes = nil
		} else if len([]rune(n)) > 2000 {
			errs.Add("notes", "must be at most 2000 characters")
		}
	}
	return t, errs
}

// GetTerms returns a client's terms (nothing set before the first save).
func (s *Service) GetTerms(ctx context.Context, p authz.Principal, companyID string) (Terms, error) {
	var out Terms
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = termsIn(ctx, tx, companyID, true)
		return err
	})
	return out, err
}

// termsIn reads a client's terms in tx; check: NOT_FOUND for a missing company.
func termsIn(ctx context.Context, tx pgx.Tx, companyID string, check bool) (Terms, error) {
	q := store.New(tx)
	if check {
		exists, err := q.CompanyExists(ctx, companyID)
		if err != nil {
			return Terms{}, err
		}
		if !exists {
			return Terms{}, apperr.NotFound("company")
		}
	}
	row, err := q.GetCompanyTerms(ctx, companyID)
	if database.IsNoRows(err) {
		return Terms{}, nil
	}
	if err != nil {
		return Terms{}, err
	}
	fee, err := database.DecimalString(row.FeePercent)
	if err != nil {
		return Terms{}, err
	}
	return Terms{
		FeePercent: trimZeros(fee), PaymentTermsDays: intOf(row.PaymentTermsDays), GuaranteeDays: intOf(row.GuaranteeDays),
		SignedOn: dateOf(row.SignedOn), Notes: row.Notes, Version: int(row.Version), UpdatedAt: &row.UpdatedAt,
	}, nil
}

// PutTerms saves a client's terms. expectedVersion is the version the person
// loaded (0: none yet).
func (s *Service) PutTerms(ctx context.Context, p authz.Principal, companyID string, expectedVersion int, in Terms) (Terms, error) {
	in, errs := NormalizeTerms(in)
	if err := errs.Err(); err != nil {
		return Terms{}, err
	}
	var out Terms
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		before, err := termsIn(ctx, tx, companyID, true)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return apperr.StaleVersion("terms").WithDetail("currentVersion", before.Version)
		}
		q := store.New(tx)
		pay, guarantee := int32Of(in.PaymentTermsDays), int32Of(in.GuaranteeDays)
		var version int32
		var at time.Time
		if expectedVersion == 0 {
			row, err := q.InsertCompanyTerms(ctx, store.InsertCompanyTermsParams{
				CompanyID: companyID, FeePercent: in.FeePercent, PaymentTermsDays: pay, GuaranteeDays: guarantee,
				SignedOn: pgDate(in.SignedOn), Notes: in.Notes,
			})
			if database.IsNoRows(err) {
				return apperr.StaleVersion("terms")
			}
			if err != nil {
				return err
			}
			version, at = row.Version, row.UpdatedAt
		} else {
			row, err := q.UpdateCompanyTerms(ctx, store.UpdateCompanyTermsParams{
				CompanyID: companyID, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small
				FeePercent: in.FeePercent, PaymentTermsDays: pay, GuaranteeDays: guarantee, SignedOn: pgDate(in.SignedOn), Notes: in.Notes,
			})
			if database.IsNoRows(err) {
				return apperr.StaleVersion("terms")
			}
			if err != nil {
				return err
			}
			version, at = row.Version, row.UpdatedAt
		}
		out = in
		out.FeePercent = trimZeros(in.FeePercent)
		out.Version, out.UpdatedAt = int(version), &at
		// The fee, payment terms and guarantee are commercial terms, not a
		// person's pay: they go in the audit log as they are.
		return audit.Record(ctx, tx, audit.Entry{Action: "company.terms_saved", EntityType: "company", EntityID: companyID,
			Changes: audit.Diff(termsFields(before), termsFields(out))})
	})
	return out, err
}

func termsFields(t Terms) map[string]any {
	day := func(d *time.Time) any {
		if d == nil {
			return nil
		}
		return d.Format(time.DateOnly)
	}
	return map[string]any{"feePercent": deref(trimZeros(t.FeePercent)), "paymentTermsDays": t.PaymentTermsDays,
		"guaranteeDays": t.GuaranteeDays, "signedOn": day(t.SignedOn), "notes": t.Notes}
}

// trimZeros writes a percentage as people do: "18.00" is "18", "12.50" "12.5".
func trimZeros(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	if strings.Contains(v, ".") {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	return &v
}

func intOf(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func int32Of(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v) //nolint:gosec // 0..365, checked
	return &n
}

func dateOf(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
