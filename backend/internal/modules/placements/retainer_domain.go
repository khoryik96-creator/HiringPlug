package placements

// A job's retainer, as plain values: stages the client pays while the search
// runs, each due on a date or some days after the retainer was signed or the
// placement starts. What the retainer adds up to is taken off the fee of the
// first placement on the job (its balance is what that placement invoices).

import (
	"math/big"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// When a retainer stage is due.
const (
	DueOnDate       = "on_date"       // on DueOn
	DueAfterSigning = "after_signing" // DueDays after the retainer was signed
	DueAfterStart   = "after_start"   // DueDays after the placement starts
)

// MaxRetainerStages is the most stages a retainer has.
const MaxRetainerStages = 12

// RetainerStage is one part of a retainer.
type RetainerStage struct {
	Label         string
	Amount        string // decimal, more than 0, in the retainer's currency
	DueKind       string
	DueOn         *time.Time // DueOnDate only
	DueDays       *int       // the others
	InvoiceStatus string
	InvoiceNumber *string
	InvoicedOn    *time.Time
	PaidOn        *time.Time
}

// Retainer is a job's retainer. Version 0: none yet.
type Retainer struct {
	Version  int
	Currency string
	SignedOn *time.Time
	Stages   []RetainerStage
}

// NormalizeRetainer tidies a retainer and checks it. Invoice dates default to
// today, as on a placement. Field errors name the stage: stages.1.amount.
func NormalizeRetainer(r Retainer, today time.Time) (Retainer, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if len(r.Stages) > MaxRetainerStages {
		errs.Add("stages", "at most 12 stages")
		return r, errs
	}
	if len(r.Stages) > 0 && !currencyCode(r.Currency) {
		errs.Add("currency", "must be a 3-letter currency code, e.g. MYR")
	}
	stages := make([]RetainerStage, len(r.Stages))
	for i, s := range r.Stages {
		at := func(field string) string { return "stages." + itoa(i) + "." + field }
		if l := trimmed(&s.Label); l == nil {
			errs.Add(at("label"), "enter a name, e.g. Engagement")
		} else if s.Label = *l; len([]rune(s.Label)) > 100 {
			errs.Add(at("label"), "is too long")
		}
		if v, ok := new(big.Rat).SetString(s.Amount); !ok || v.Sign() <= 0 {
			errs.Add(at("amount"), "must be more than 0")
		} else {
			s.Amount = v.FloatString(2)
		}
		switch s.DueKind {
		case DueOnDate:
			s.DueDays = nil
			if s.DueOn == nil {
				errs.Add(at("dueOn"), "enter the day it is due")
			}
		case DueAfterSigning, DueAfterStart:
			s.DueOn = nil
			if s.DueDays == nil || *s.DueDays < 0 || *s.DueDays > 730 {
				errs.Add(at("dueDays"), "must be between 0 and 730 days")
			}
		default:
			errs.Add(at("dueKind"), "must be on a date, after signing or after the start")
		}
		s.InvoiceNumber = trimmed(s.InvoiceNumber)
		switch s.InvoiceStatus {
		case "", NotInvoiced:
			s.InvoiceStatus, s.InvoiceNumber, s.InvoicedOn, s.PaidOn = NotInvoiced, nil, nil, nil
		case Invoiced:
			s.PaidOn = nil
			if s.InvoicedOn == nil {
				s.InvoicedOn = &today
			}
		case Paid:
			if s.PaidOn == nil {
				s.PaidOn = &today
			}
			if s.InvoicedOn == nil {
				s.InvoicedOn = s.PaidOn
			}
			if s.PaidOn.Before(*s.InvoicedOn) {
				errs.Add(at("paidOn"), "is before the invoice date")
			}
		default:
			errs.Add(at("invoiceStatus"), "must be not invoiced, invoiced or paid")
		}
		if s.InvoiceNumber != nil && len([]rune(*s.InvoiceNumber)) > 50 {
			errs.Add(at("invoiceNumber"), "is too long")
		}
		stages[i] = s
	}
	r.Stages = stages
	return r, errs
}

// DueDay is the day a stage is due: its date, or the days after signing or
// after start (nil while that day is not known).
func (r Retainer) DueDay(s RetainerStage, start *time.Time) *time.Time {
	from := start
	switch s.DueKind {
	case DueOnDate:
		return s.DueOn
	case DueAfterSigning:
		from = r.SignedOn
	}
	if from == nil || s.DueDays == nil {
		return nil
	}
	d := from.AddDate(0, 0, *s.DueDays)
	return &d
}

// Total is what the stages add up to.
func (r Retainer) Total() string {
	sum := new(big.Rat)
	for _, s := range r.Stages {
		if v, ok := new(big.Rat).SetString(s.Amount); ok {
			sum.Add(sum, v)
		}
	}
	return sum.FloatString(2)
}

// Balance is what is left of a fee once the retainer is taken off, never
// below 0; false when the two are in different currencies (nothing is taken
// off then).
func (r Retainer) Balance(fee, currency string) (string, bool) {
	if len(r.Stages) == 0 || currency != r.Currency {
		return fee, false
	}
	f, ok := new(big.Rat).SetString(fee)
	if !ok {
		return fee, false
	}
	t, _ := new(big.Rat).SetString(r.Total())
	f.Sub(f, t)
	if f.Sign() < 0 {
		f.SetInt64(0)
	}
	return f.FloatString(2), true
}

func currencyCode(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func itoa(i int) string { return big.NewInt(int64(i)).String() }
