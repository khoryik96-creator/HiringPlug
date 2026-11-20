package claims

// The rules of expense claims, as pure functions on plain values.

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// A claim's status: draft → submitted → approved or rejected; approved →
// paid. A rejected claim is changed and submitted again.
const (
	Draft     = "draft"
	Submitted = "submitted"
	Approved  = "approved"
	Rejected  = "rejected"
	Paid      = "paid"
)

// How reading the receipt went.
const (
	OcrReading = "reading"
	OcrDone    = "done"
	OcrFailed  = "failed"
)

// readingPatience is how long a receipt may take to read. Past it the claim
// shows as not read (the worker is down or its job died), so the person fills
// it in; a reading that still arrives fills only the fields left empty.
const readingPatience = 10 * time.Minute

// tooLongReading is the message for a receipt still unread after readingPatience.
const tooLongReading = "The receipt could not be read in time: fill it in."

// OcrShown is how reading the receipt went, as the person sees it at now.
func OcrShown(status string, problem *string, uploaded, now time.Time) (string, *string) {
	if status == OcrReading && now.Sub(uploaded) > readingPatience {
		msg := tooLongReading
		return OcrFailed, &msg
	}
	return status, problem
}

// What a claim may be tagged to: a client (company or contact) or candidate.
const (
	LinkCompany   = "company"
	LinkContact   = "contact"
	LinkCandidate = "candidate"
)

// The fields a person fills in, as the contract names them.
const (
	FieldReceiptDate = "receiptDate"
	FieldAmount      = "amount"
	FieldCurrency    = "currency"
	FieldMerchant    = "merchant"
	FieldReceiptType = "receiptType"
	FieldClaimType   = "claimType"
	FieldPurpose     = "purpose"
	FieldLink        = "link"
)

// DefaultCurrency is what a receipt is in unless it says otherwise.
const DefaultCurrency = "MYR"

// Limits on what is written.
const (
	MaxMerchant = 200
	MaxPurpose  = 500
	MaxNote     = 500
	// MaxAmount is the most one receipt may claim.
	MaxAmount = 1_000_000
)

// Values are what a claim claims: the fields its claimant fills in.
type Values struct {
	ReceiptDate *time.Time // a day, at midnight UTC
	Amount      *string    // e.g. "23.50"
	Currency    string
	Merchant    *string
	ReceiptType *string // a claim_receipt_type option
	ClaimType   *string // a claim_type option
	Purpose     *string
	LinkKind    *string
	LinkID      *string
}

// Read is what was read from a receipt (nil: not found on it).
type Read struct {
	ReceiptDate *time.Time
	Amount      *string
	Currency    *string
	Merchant    *string
	ReceiptType *string
}

// Editable reports whether the claimant may change, delete or submit a claim
// in this status.
func Editable(status string) bool { return status == Draft || status == Rejected }

// Missing lists what must still be filled in before a claim can be submitted.
func Missing(v Values) []string {
	out := []string{}
	if v.ReceiptDate == nil {
		out = append(out, FieldReceiptDate)
	}
	if v.Amount == nil || isZero(*v.Amount) {
		out = append(out, FieldAmount)
	}
	if blank(v.ReceiptType) {
		out = append(out, FieldReceiptType)
	}
	if blank(v.ClaimType) {
		out = append(out, FieldClaimType)
	}
	if blank(v.Purpose) {
		out = append(out, FieldPurpose)
	}
	return out
}

// Labels names the fields for people ("Fill in the purpose").
var Labels = map[string]string{
	FieldReceiptDate: "receipt date", FieldAmount: "amount", FieldCurrency: "currency", FieldMerchant: "merchant",
	FieldReceiptType: "receipt type", FieldClaimType: "claim type", FieldPurpose: "purpose", FieldLink: "client or candidate",
}

func blank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

var amountRe = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,2})?$`)

// CleanAmount checks an amount ("23.5", "1200") and writes it with two
// decimals ("23.50"); ok is false when it is not a positive amount up to
// MaxAmount.
func CleanAmount(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !amountRe.MatchString(s) || isZero(s) {
		return "", false
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(whole) > 7 || len(whole) == 7 && whole != "1000000" || whole == "1000000" && strings.Trim(frac, "0") != "" {
		return "", false
	}
	return whole + "." + (frac + "00")[:2], true
}

func isZero(s string) bool { return strings.Trim(strings.ReplaceAll(s, ".", ""), "0") == "" }

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidCurrency reports an ISO 4217 code's shape (MYR, SGD...).
func ValidCurrency(s string) bool { return currencyRe.MatchString(s) }

// ValidReceiptDate reports a day a receipt may carry: not after tomorrow
// (time zones) and not before 2000.
func ValidReceiptDate(day, today time.Time) bool {
	return !day.After(today.AddDate(0, 0, 1)) && day.Year() >= 2000
}

// CleanText trims a person's text and squeezes its spaces; nil when nothing
// is left. ok is false when it is longer than max characters.
func CleanText(s *string, maxLen int) (out *string, ok bool) {
	if s == nil {
		return nil, true
	}
	t := strings.Join(strings.Fields(*s), " ")
	if t == "" {
		return nil, true
	}
	if utf8.RuneCountInString(t) > maxLen {
		return nil, false
	}
	return &t, true
}

var linkPrefix = map[string]string{LinkCompany: "comp_", LinkContact: "cont_", LinkCandidate: "cand_"}

// ValidLink reports a client or candidate reference whose id fits its kind.
func ValidLink(kind, id string) bool {
	prefix, ok := linkPrefix[kind]
	return ok && strings.HasPrefix(id, prefix) && len(id) == len(prefix)+26
}

// ClaimTypeFor is the claim type a receipt type suggests (built-in options
// only; "" for none): fares, parking, fuel and trips are travel, phone bills
// and office supplies office expenses. Food and drinks could be a client's
// lunch or a team's: the claimant says.
func ClaimTypeFor(receiptType string) string {
	switch receiptType {
	case "grab_fare", "parking_toll", "fuel", "travel":
		return "travel"
	case "phone", "office":
		return "office"
	}
	return ""
}

// Prefill puts what was read from a receipt into the fields still empty (a
// person's value is never replaced), and returns the values and the fields
// filled. The currency, which is never empty, is taken only while the claim
// is untouched (fresh), as its default was nobody's choice.
func Prefill(v Values, r Read, fresh bool) (Values, []string) {
	var filled []string
	if v.ReceiptDate == nil && r.ReceiptDate != nil {
		v.ReceiptDate = r.ReceiptDate
		filled = append(filled, FieldReceiptDate)
	}
	if v.Amount == nil && r.Amount != nil {
		v.Amount = r.Amount
		filled = append(filled, FieldAmount)
	}
	if fresh && r.Currency != nil && *r.Currency != v.Currency {
		v.Currency = *r.Currency
		filled = append(filled, FieldCurrency)
	}
	if blank(v.Merchant) && r.Merchant != nil {
		v.Merchant = r.Merchant
		filled = append(filled, FieldMerchant)
	}
	if blank(v.ReceiptType) && r.ReceiptType != nil {
		v.ReceiptType = r.ReceiptType
		filled = append(filled, FieldReceiptType)
	}
	return v, filled
}

// Unmark is fromReceipt without the fields a person has now changed: they
// are no longer what the receipt said.
func Unmark(fromReceipt, changed []string) []string {
	out := []string{}
	for _, f := range fromReceipt {
		if !slices.Contains(changed, f) {
			out = append(out, f)
		}
	}
	return out
}

// Marked is fromReceipt with the fields just filled from the receipt.
func Marked(fromReceipt, filled []string) []string {
	out := slices.Clone(fromReceipt)
	if out == nil {
		out = []string{}
	}
	for _, f := range filled {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// Decisions an approver takes.
const (
	Approve = "approve"
	Reject  = "reject"
	Pay     = "pay"
)

// Decide is the status a decision moves a claim in this status to; ok is
// false when it cannot: only submitted claims are approved or rejected, only
// approved ones paid.
func Decide(decision, status string) (to string, ok bool) {
	switch {
	case decision == Approve && status == Submitted:
		return Approved, true
	case decision == Reject && status == Submitted:
		return Rejected, true
	case decision == Pay && status == Approved:
		return Paid, true
	}
	return "", false
}
