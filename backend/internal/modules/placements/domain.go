package placements

// The placement rules, as plain functions on plain values: no database, no HTTP.

import (
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Statuses and invoice statuses.
const (
	Active      = "active"
	FellThrough = "fell_through"
	Cancelled   = "cancelled"

	NoReplacement   = "none"
	ReplacementOwed = "owed"

	NotInvoiced = "not_invoiced"
	Invoiced    = "invoiced"
	Paid        = "paid"
)

// DefaultGuaranteeDays is the guarantee period when none is given.
const DefaultGuaranteeDays = 90

// Details are a placement's facts (all but salary and fee).
type Details struct {
	RecruiterID   *string
	StartDate     time.Time // a day: midnight UTC
	GuaranteeDays int
	Status        string
	LeftOn        *time.Time
	StatusNote    *string
	Replacement   string // after a fall-through: owed, provided or refunded; none otherwise
	InvoiceStatus string
	InvoiceNumber *string
	InvoicedOn    *time.Time
	PaidOn        *time.Time
	Sheet
}

// GuaranteeEnds is the last day of the guarantee period.
func (d Details) GuaranteeEnds() time.Time { return d.StartDate.AddDate(0, 0, d.GuaranteeDays) }

// Patch is a partial update of Details.
type Patch struct {
	RecruiterID   apiconv.Change[*string]
	StartDate     apiconv.Change[time.Time]
	GuaranteeDays apiconv.Change[int]
	Status        apiconv.Change[string]
	LeftOn        apiconv.Change[*time.Time]
	StatusNote    apiconv.Change[*string]
	Replacement   apiconv.Change[string]
	InvoiceStatus apiconv.Change[string]
	InvoiceNumber apiconv.Change[*string]
	InvoicedOn    apiconv.Change[*time.Time]
	PaidOn        apiconv.Change[*time.Time]
	Sheet         SheetPatch
}

// Apply returns d with the patch's changes.
func (p Patch) Apply(d Details) Details {
	apiconv.Apply(&d.RecruiterID, p.RecruiterID)
	apiconv.Apply(&d.StartDate, p.StartDate)
	apiconv.Apply(&d.GuaranteeDays, p.GuaranteeDays)
	apiconv.Apply(&d.Status, p.Status)
	apiconv.Apply(&d.LeftOn, p.LeftOn)
	apiconv.Apply(&d.StatusNote, p.StatusNote)
	apiconv.Apply(&d.Replacement, p.Replacement)
	apiconv.Apply(&d.InvoiceStatus, p.InvoiceStatus)
	apiconv.Apply(&d.InvoiceNumber, p.InvoiceNumber)
	apiconv.Apply(&d.InvoicedOn, p.InvoicedOn)
	apiconv.Apply(&d.PaidOn, p.PaidOn)
	d.Sheet = p.Sheet.Apply(d.Sheet)
	return d
}

// Normalize tidies a placement and checks it. Field problems come back as
// field errors; a fall-through after the guarantee ended comes back as
// problem (the placement stands: it is not a fall-through). Invoice dates
// default to today.
//   - only a fall-through has the day the candidate left, within the guarantee,
//     and a replacement (owed until provided or refunded);
//   - not invoiced has no invoice number or dates; invoiced has the day it was
//     invoiced; paid also has the day it was paid, not before the invoice.
func Normalize(d Details, today time.Time) (Details, apperr.FieldErrors, string) {
	var errs apperr.FieldErrors
	d.StatusNote, d.InvoiceNumber = trimmed(d.StatusNote), trimmed(d.InvoiceNumber)
	if d.Status == "" {
		d.Status = Active
	}
	if d.InvoiceStatus == "" {
		d.InvoiceStatus = NotInvoiced
	}
	if d.GuaranteeDays < 0 || d.GuaranteeDays > 365 {
		errs.Add("guaranteeDays", "must be between 0 and 365 days")
	}
	d.Sheet = d.normalize(d.StartDate, &errs)
	problem := ""
	if d.Status != FellThrough {
		d.LeftOn, d.Replacement = nil, NoReplacement
	} else {
		if d.Replacement == "" || d.Replacement == NoReplacement {
			d.Replacement = ReplacementOwed // a fall-through within the guarantee: the client is owed one
		}
		switch {
		case d.LeftOn == nil:
			errs.Add("leftOn", "enter the day the candidate left")
		case d.LeftOn.Before(d.StartDate):
			errs.Add("leftOn", "is before the start date")
		case d.LeftOn.After(d.GuaranteeEnds()):
			problem = "They left after the guarantee period ended on " + d.GuaranteeEnds().Format("2 January 2006") +
				": the placement stands, it did not fall through."
		}
	}
	switch d.InvoiceStatus {
	case NotInvoiced:
		d.InvoiceNumber, d.InvoicedOn, d.PaidOn = nil, nil, nil
	case Invoiced:
		d.PaidOn = nil
		if d.InvoicedOn == nil {
			d.InvoicedOn = &today
		}
	case Paid:
		if d.PaidOn == nil {
			d.PaidOn = &today
		}
		if d.InvoicedOn == nil {
			d.InvoicedOn = d.PaidOn
		}
		if d.PaidOn.Before(*d.InvoicedOn) {
			errs.Add("paidOn", "is before the invoice date")
		}
	}
	return d, errs, problem
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// Salary is an agreed salary.
type Salary struct{ Amount, Currency, Period string }

// Money is a placement's salary and fee. The fee is a percentage of the
// yearly salary (FeeAmount worked out) or a fixed amount (FeePercent nil).
type Money struct {
	Salary      *Salary
	FeePercent  *string
	FeeAmount   *string
	FeeCurrency *string
}

// Empty reports whether nothing is recorded.
func (m Money) Empty() bool { return m.Salary == nil && m.FeeAmount == nil && m.FeePercent == nil }

// NormalizeMoney checks the salary and fee and works out a percentage fee:
// the yearly salary (a monthly salary counts 12 times) times the percentage,
// to the cent, in the salary's currency.
func NormalizeMoney(m Money) (Money, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if m.FeePercent == nil {
		if (m.FeeAmount == nil) != (m.FeeCurrency == nil) {
			errs.Add("money.feeAmount", "give the fee amount with its currency")
		}
		return m, errs
	}
	pct, ok := new(big.Rat).SetString(*m.FeePercent)
	switch {
	case !ok || pct.Sign() <= 0 || pct.Cmp(big.NewRat(100, 1)) > 0:
		errs.Add("money.feePercent", "must be more than 0 and at most 100")
	case m.Salary == nil:
		errs.Add("money.feePercent", "a percentage needs the salary; or enter the fee amount instead")
	case m.Salary.Period != "month" && m.Salary.Period != "year":
		errs.Add("money.feePercent", "a percentage needs a monthly or yearly salary; or enter the fee amount instead")
	}
	if len(errs) > 0 {
		return m, errs
	}
	yearly, ok := new(big.Rat).SetString(m.Salary.Amount)
	if !ok {
		errs.Add("money.salary.amount", "is not an amount")
		return m, errs
	}
	if m.Salary.Period == "month" {
		yearly.Mul(yearly, big.NewRat(12, 1))
	}
	fee := new(big.Rat).Mul(yearly, pct)
	fee.Quo(fee, big.NewRat(100, 1))
	amount, currency := fee.FloatString(2), m.Salary.Currency
	m.FeeAmount, m.FeeCurrency = &amount, &currency
	return m, errs
}

// FeeLine is what Totals needs to know about one placement.
type FeeLine struct {
	Status, InvoiceStatus string
	Amount, Currency      string
}

// FeeTotal is the fees in one currency, by invoice status.
type FeeTotal struct {
	Currency                           string
	Total, NotInvoiced, Invoiced, Paid string
}

// Totals adds up the fees of placements that stand (not fallen through or
// cancelled), per currency, sorted by currency.
func Totals(lines []FeeLine) []FeeTotal {
	type sums struct{ total, notInvoiced, invoiced, paid big.Rat }
	by := map[string]*sums{}
	for _, l := range lines {
		amount, ok := new(big.Rat).SetString(l.Amount)
		if l.Status != Active || !ok {
			continue
		}
		s := by[l.Currency]
		if s == nil {
			s = &sums{}
			by[l.Currency] = s
		}
		s.total.Add(&s.total, amount)
		switch l.InvoiceStatus {
		case Paid:
			s.paid.Add(&s.paid, amount)
		case Invoiced:
			s.invoiced.Add(&s.invoiced, amount)
		default:
			s.notInvoiced.Add(&s.notInvoiced, amount)
		}
	}
	out := make([]FeeTotal, 0, len(by))
	for currency, s := range by {
		out = append(out, FeeTotal{
			Currency: currency, Total: s.total.FloatString(2),
			NotInvoiced: s.notInvoiced.FloatString(2), Invoiced: s.invoiced.FloatString(2), Paid: s.paid.FloatString(2),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}
