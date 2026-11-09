package companies

// A client's terms of business: rates by annual salary, each with the fee (a
// percentage of the yearly salary), payment terms, guarantee and remarks
// (another payment method), which fill in a new placement.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
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

// Terms are a client's terms of business: their rates by annual salary.
type Terms struct {
	Tiers          []Tier
	SalaryCurrency string // of the tiers' salary limits
	SignedOn       *time.Time
	Notes          *string
	Version        int // 0 before the first save
	UpdatedAt      *time.Time
}

// Tier is one rate: for annual salaries from SalaryFrom (inclusive) up to
// SalaryBelow (exclusive); nil limits are open. Nil fields are not set.
type Tier struct {
	SalaryFrom       *string `json:"salaryFrom,omitempty"`
	SalaryBelow      *string `json:"salaryBelow,omitempty"`
	FeePercent       *string `json:"feePercent,omitempty"` // e.g. "18"
	PaymentTermsDays *int    `json:"paymentTermsDays,omitempty"`
	GuaranteeDays    *int    `json:"guaranteeDays,omitempty"`
	Remarks          *string `json:"remarks,omitempty"` // another payment method, anything else
}

func (t Tier) empty() bool {
	return t.SalaryFrom == nil && t.SalaryBelow == nil && t.FeePercent == nil && t.PaymentTermsDays == nil &&
		t.GuaranteeDays == nil && t.Remarks == nil
}

// NormalizeTerms checks terms a person saves: rows left empty are dropped;
// each rate's salary range must not overlap another's.
func NormalizeTerms(t Terms) (Terms, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	t.SalaryCurrency = strings.ToUpper(strings.TrimSpace(t.SalaryCurrency))
	if t.SalaryCurrency == "" {
		t.SalaryCurrency = "MYR"
	}
	// Each rate keeps the number it was sent with, so an error names the row
	// the person sees (empty rows included), named as the contract names them.
	type numbered struct {
		Tier
		row int
	}
	var tiers []numbered
	for row, tier := range t.Tiers {
		if tier.Remarks != nil {
			r := strings.TrimSpace(*tier.Remarks)
			tier.Remarks = &r
			if r == "" {
				tier.Remarks = nil
			}
		}
		tier.FeePercent = trimZeros(tier.FeePercent)
		tier.SalaryFrom, tier.SalaryBelow = trimZeros(tier.SalaryFrom), trimZeros(tier.SalaryBelow)
		if !tier.empty() {
			tiers = append(tiers, numbered{tier, row})
		}
	}
	if len(tiers) > 20 {
		errs.Add("tiers", "at most 20 rates")
	}
	for i, tier := range tiers {
		at := func(f string) string { return "tiers." + strconv.Itoa(tier.row) + "." + f }
		if tier.FeePercent != nil {
			pct, ok := new(big.Rat).SetString(*tier.FeePercent)
			if !ok || pct.Sign() <= 0 || pct.Cmp(big.NewRat(100, 1)) > 0 {
				errs.Add(at("feePercent"), "a percentage above 0, at most 100")
			}
		}
		for _, d := range []struct {
			field string
			v     *int
		}{{"guaranteeDays", tier.GuaranteeDays}, {"paymentTermsDays", tier.PaymentTermsDays}} {
			if d.v != nil && (*d.v < 0 || *d.v > 365) {
				errs.Add(at(d.field), "between 0 and 365 days")
			}
		}
		if tier.Remarks != nil && len([]rune(*tier.Remarks)) > 500 {
			errs.Add(at("remarks"), "must be at most 500 characters")
		}
		from, below := ratOf(tier.SalaryFrom), ratOf(tier.SalaryBelow)
		if from != nil && below != nil && from.Cmp(below) >= 0 {
			errs.Add(at("salaryBelow"), "must be more than the salary it starts from")
			continue
		}
		for _, other := range tiers[:i] {
			if overlap(other.Tier, tier.Tier) {
				errs.Add(at("salaryFrom"), "overlaps rate "+strconv.Itoa(other.row+1)+": each salary must fall in one rate only")
				break
			}
		}
	}
	t.Tiers = make([]Tier, len(tiers))
	for i, tier := range tiers {
		t.Tiers[i] = tier.Tier
	}
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

// overlap reports whether two rates' salary ranges share a salary.
func overlap(a, b Tier) bool {
	below := func(x, y *big.Rat) bool { return x != nil && y != nil && x.Cmp(y) <= 0 } // [.., x) ends before [y, ..)
	aFrom, aBelow, bFrom, bBelow := ratOf(a.SalaryFrom), ratOf(a.SalaryBelow), ratOf(b.SalaryFrom), ratOf(b.SalaryBelow)
	return !below(aBelow, bFrom) && !below(bBelow, aFrom)
}

func ratOf(s *string) *big.Rat {
	if s == nil {
		return nil
	}
	r, ok := new(big.Rat).SetString(*s)
	if !ok {
		return nil
	}
	return r
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
	var tiers []Tier
	if err := json.Unmarshal(row.Tiers, &tiers); err != nil {
		return Terms{}, err
	}
	for i, t := range tiers { // "18.00" (terms saved before tiers) reads "18"
		tiers[i].FeePercent, tiers[i].SalaryFrom, tiers[i].SalaryBelow = trimZeros(t.FeePercent), trimZeros(t.SalaryFrom), trimZeros(t.SalaryBelow)
	}
	return Terms{
		Tiers: tiers, SalaryCurrency: row.SalaryCurrency, SignedOn: dateOf(row.SignedOn), Notes: row.Notes,
		Version: int(row.Version), UpdatedAt: &row.UpdatedAt,
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
		tiers, err := json.Marshal(in.Tiers)
		if err != nil {
			return err
		}
		var version int32
		var at time.Time
		if expectedVersion == 0 {
			row, err := q.InsertCompanyTerms(ctx, store.InsertCompanyTermsParams{
				CompanyID: companyID, Tiers: tiers, SalaryCurrency: in.SalaryCurrency, SignedOn: pgDate(in.SignedOn), Notes: in.Notes,
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
				Tiers: tiers, SalaryCurrency: in.SalaryCurrency, SignedOn: pgDate(in.SignedOn), Notes: in.Notes,
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
		out.Version, out.UpdatedAt = int(version), &at
		// The rates are commercial terms, not a person's pay (the salary
		// limits are the client's bands): they go in the audit log as they are.
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
	rates := make([]string, len(t.Tiers))
	for i, r := range t.Tiers {
		rates[i] = fmt.Sprintf("salary %s to below %s: %s%% · %s days to pay · %s-day guarantee", deref(r.SalaryFrom), deref(r.SalaryBelow),
			deref(r.FeePercent), intText(r.PaymentTermsDays), intText(r.GuaranteeDays))
	}
	return map[string]any{"rates": rates, "salaryCurrency": t.SalaryCurrency,
		"signedOn": day(t.SignedOn), "notes": t.Notes}
}

func intText(v *int) string {
	if v == nil {
		return "?"
	}
	return strconv.Itoa(*v)
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
