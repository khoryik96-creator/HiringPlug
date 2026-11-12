package placements

// Invoices, as pure functions: the agency's template and its defaults, the
// {{fields}} it fills, the numbering, the due date, tax and totals, and the
// invoice laid out as a document.

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

// DefaultPaymentTermsDays is a placement's payment terms when its client's
// terms of business set none.
const DefaultPaymentTermsDays = 30

// InvoiceSettings are the agency's invoice template and the email that
// sends it.
type InvoiceSettings struct {
	Issuer, BankDetails     string
	NumberPrefix            string
	NextNumber              int
	TaxLabel, TaxPercent    string
	LineText, Footer        string
	EmailSubject, EmailBody string
	SendHour                int
	Version                 int // 0: the defaults (nothing saved yet)
}

// DefaultInvoiceSettings are the template an agency starts with.
func DefaultInvoiceSettings(agency string) InvoiceSettings {
	return InvoiceSettings{
		Issuer: agency, NumberPrefix: "INV-", NextNumber: 1, TaxLabel: "SST", TaxPercent: "0",
		LineText:     "Recruitment fee: {{candidate.name}}, {{job.title}} (starting {{placement.startDate}})\n{{fee.basis}}",
		Footer:       "Payment is due within {{invoice.terms}} days, by {{invoice.dueDate}}. Thank you for your business.",
		EmailSubject: "Invoice {{invoice.number}}: {{candidate.name}}, {{job.title}}",
		EmailBody: "Dear {{contact.firstName}},\n\n{{candidate.name}} starts with {{client.name}} today as {{job.title}}: congratulations!\n\n" +
			"Please find attached invoice {{invoice.number}} for {{invoice.total}}, due on {{invoice.dueDate}}.\n\n" +
			"Thank you,\n{{agency.name}}",
		SendHour: 9,
	}
}

// InvoiceField is a {{field}} the texts may use.
type InvoiceField struct{ Key, Label, Example string }

// InvoiceFields are the fields, with an example of each.
var InvoiceFields = []InvoiceField{
	{"agency.name", "Your agency", "Hyppies"},
	{"client.name", "Client", "Maju Foods Berhad"},
	{"contact.name", "Invoice recipient", "Aisyah Rahman"},
	{"contact.firstName", "Recipient's first name", "Aisyah"},
	{"candidate.name", "Candidate", "Siti Nur Aisyah"},
	{"job.title", "Job", "Finance Manager"},
	{"placement.startDate", "Start date", "1 December 2026"},
	{"placement.poNumber", "PO number", "PO-88123"},
	{"invoice.number", "Invoice number", "INV-0042"},
	{"invoice.date", "Invoice date", "1 December 2026"},
	{"invoice.dueDate", "Due date", "31 December 2026"},
	{"invoice.terms", "Payment terms (days)", "30"},
	{"invoice.subtotal", "Fee before tax", "MYR 21,600.00"},
	{"invoice.tax", "Tax", "MYR 1,728.00"},
	{"invoice.total", "Total", "MYR 23,328.00"},
	{"fee.percent", "Fee percentage", "18%"},
	{"fee.basis", "How the fee is worked out", "18% of the annual salary of MYR 120,000.00"},
	{"salary.annual", "Annual salary", "MYR 120,000.00"},
}

var fieldRe = regexp.MustCompile(`\{\{\s*([a-zA-Z]+\.[a-zA-Z]+)\s*\}\}`)

// FillInvoice puts values into a text's {{fields}}; unknown fields stay as written.
func FillInvoice(text string, values map[string]string) string {
	return fieldRe.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := values[fieldRe.FindStringSubmatch(m)[1]]; ok {
			return v
		}
		return m
	})
}

// NormalizeInvoiceSettings checks a template a person saves.
func NormalizeInvoiceSettings(s InvoiceSettings) (InvoiceSettings, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	tidy := func(v string) string { return strings.TrimSpace(strings.ReplaceAll(v, "\r\n", "\n")) }
	s.Issuer, s.BankDetails, s.LineText, s.Footer, s.EmailBody = tidy(s.Issuer), tidy(s.BankDetails), tidy(s.LineText), tidy(s.Footer), tidy(s.EmailBody)
	s.NumberPrefix, s.TaxLabel = strings.TrimSpace(s.NumberPrefix), strings.TrimSpace(s.TaxLabel)
	s.EmailSubject = strings.Join(strings.Fields(s.EmailSubject), " ")
	if s.Issuer == "" {
		errs.Add("issuer", "is required: your company's name at least")
	}
	if s.LineText == "" {
		errs.Add("lineText", "is required")
	}
	if s.EmailSubject == "" {
		errs.Add("emailSubject", "is required")
	}
	if s.EmailBody == "" {
		errs.Add("emailBody", "is required")
	}
	if pct, ok := new(big.Rat).SetString(s.TaxPercent); !ok || pct.Sign() < 0 || pct.Cmp(big.NewRat(100, 1)) > 0 {
		errs.Add("taxPercent", "a percentage from 0 to 100")
	}
	known := map[string]bool{}
	for _, f := range InvoiceFields {
		known[f.Key] = true
	}
	for field, text := range map[string]string{"lineText": s.LineText, "footer": s.Footer, "emailSubject": s.EmailSubject, "emailBody": s.EmailBody} {
		for _, m := range fieldRe.FindAllStringSubmatch(text, -1) {
			if !known[m[1]] {
				errs.Add(field, fmt.Sprintf("{{%s}} is not a field", m[1]))
				break
			}
		}
	}
	return s, errs
}

// InvoiceNumber is a number as written on an invoice: INV-0042.
func InvoiceNumber(prefix string, n int) string { return fmt.Sprintf("%s%04d", prefix, n) }

// DueOn is when payment is due: set by someone, or the invoice date plus
// the payment terms.
func DueOn(invoiceDate time.Time, termsDays int, set *time.Time) time.Time {
	if set != nil && !set.Before(invoiceDate) { // a date set by hand, unless the invoice went out after it
		return *set
	}
	return invoiceDate.AddDate(0, 0, termsDays)
}

// SendsAt is when a placement's invoice goes out on its start date: at the
// agency's hour in its time zone, or at once if that is past.
func SendsAt(start time.Time, hour int, loc *time.Location, now time.Time) time.Time {
	at := SendTime(start, hour, loc)
	if at.Before(now) {
		return now.Truncate(time.Second)
	}
	return at
}

// SendTime is the agency's hour on the start date, in its time zone.
func SendTime(start time.Time, hour int, loc *time.Location) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day(), hour, 0, 0, 0, loc)
}

// InvoiceTotals are an invoice's amounts, worked out exactly, two decimals each.
type InvoiceTotals struct{ Subtotal, Tax, Total string }

// InvoiceAmounts works out the tax on a fee and the total.
func InvoiceAmounts(fee, taxPercent string) (InvoiceTotals, error) {
	f, ok := new(big.Rat).SetString(fee)
	if !ok {
		return InvoiceTotals{}, fmt.Errorf("fee %q is not a number", fee)
	}
	pct, ok := new(big.Rat).SetString(taxPercent)
	if !ok {
		return InvoiceTotals{}, fmt.Errorf("tax %q is not a number", taxPercent)
	}
	tax := new(big.Rat).Mul(f, pct)
	tax.Quo(tax, big.NewRat(100, 1))
	taxS := tax.FloatString(2) // rounds half away from zero
	t, _ := new(big.Rat).SetString(taxS)
	return InvoiceTotals{Subtotal: f.FloatString(2), Tax: taxS, Total: new(big.Rat).Add(f, t).FloatString(2)}, nil
}

// MoneyText writes an amount as an invoice shows it: MYR 21,600.00.
func MoneyText(currency, amount string) string {
	whole, cents, _ := strings.Cut(amount, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if cents != "" {
		out += "." + (cents + "00")[:2]
	} else {
		out += ".00"
	}
	if neg {
		out = "-" + out
	}
	return strings.TrimSpace(currency + " " + out)
}

// LongDate writes a day as invoices do: 1 December 2026.
func LongDate(t time.Time) string { return t.Format("2 January 2006") }

// InvoiceDoc lays an invoice out: who it is from, its number and dates, who
// it is to, the fee line and totals, how to pay and the footer.
func InvoiceDoc(s InvoiceSettings, values map[string]string, billTo []string, taxShown bool) simpledoc.Doc {
	doc := simpledoc.Doc{{Kind: simpledoc.Title, Text: "INVOICE"}}
	if s.Issuer != "" {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Muted, Text: s.Issuer})
	}
	doc = append(doc,
		simpledoc.Block{Kind: simpledoc.Row, Cells: []string{"Invoice no.", values["invoice.number"]}},
		simpledoc.Block{Kind: simpledoc.Row, Cells: []string{"Date", values["invoice.date"]}},
		simpledoc.Block{Kind: simpledoc.Row, Cells: []string{"Due", values["invoice.dueDate"]}},
		simpledoc.Block{Kind: simpledoc.Row, Cells: []string{"Bill to", strings.Join(billTo, "\n")}},
		simpledoc.Block{Kind: simpledoc.HeaderRow, Cells: []string{"Description", "Amount"}},
		simpledoc.Block{Kind: simpledoc.Row, Cells: []string{FillInvoice(s.LineText, values), values["invoice.subtotal"]}},
	)
	if taxShown {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Row, Cells: []string{strings.TrimSpace(s.TaxLabel + " " + trimZeros(s.TaxPercent) + "%"), values["invoice.tax"]}})
	}
	doc = append(doc, simpledoc.Block{Kind: simpledoc.HeaderRow, Cells: []string{"Total", values["invoice.total"]}})
	if s.BankDetails != "" {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Payment"}, simpledoc.Block{Kind: simpledoc.Paragraph, Text: s.BankDetails})
	}
	if s.Footer != "" {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Paragraph, Text: FillInvoice(s.Footer, values)})
	}
	return doc
}

// trimZeros writes a percentage as people do: "8.00" is "8".
func trimZeros(v string) string {
	if strings.Contains(v, ".") {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	return v
}

// annualSalary is a salary for a year: a monthly one counts 12 times.
func annualSalary(s Salary) string {
	amount, ok := new(big.Rat).SetString(s.Amount)
	if !ok {
		return s.Amount
	}
	if s.Period == "month" {
		amount.Mul(amount, big.NewRat(12, 1))
	}
	return amount.FloatString(2)
}

// For is the rate for a salary: the one whose range has its annual amount
// (when it is in the terms' currency). Without a salary, or one in another
// currency, it is the only rate, when that has no salary limits.
func (t ClientTerms) For(salary *Salary) (ClientTier, bool) {
	if salary != nil && strings.EqualFold(salary.Currency, t.SalaryCurrency) {
		if annual, ok := new(big.Rat).SetString(annualSalary(*salary)); ok {
			for _, tier := range t.Tiers {
				if inRange(annual, tier.SalaryFrom, tier.SalaryBelow) {
					return tier, true
				}
			}
			return ClientTier{}, false
		}
	}
	if len(t.Tiers) == 1 && t.Tiers[0].SalaryFrom == nil && t.Tiers[0].SalaryBelow == nil {
		return t.Tiers[0], true
	}
	return ClientTier{}, false
}

func inRange(v *big.Rat, from, below *string) bool {
	if from != nil {
		if f, ok := new(big.Rat).SetString(*from); ok && v.Cmp(f) < 0 {
			return false
		}
	}
	if below != nil {
		if b, ok := new(big.Rat).SetString(*below); ok && v.Cmp(b) >= 0 {
			return false
		}
	}
	return true
}
