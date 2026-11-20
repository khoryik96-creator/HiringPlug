package claims

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

func day(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestAmountsAreCheckedAndWrittenWithTwoDecimals(t *testing.T) {
	for in, want := range map[string]string{"23.5": "23.50", "1200": "1200.00", " 7.05 ": "7.05", "1000000": "1000000.00", "0.10": "0.10"} {
		got, ok := CleanAmount(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "0", "0.00", "-5", "12.345", "1,200.00", "RM 12", "1000000.01", "2000000", "abc"} {
		_, ok := CleanAmount(in)
		assert.False(t, ok, in)
	}
}

func TestWhatIsMissingBeforeSubmitting(t *testing.T) {
	assert.Equal(t, []string{FieldReceiptDate, FieldAmount, FieldReceiptType, FieldClaimType, FieldPurpose}, Missing(Values{Currency: "MYR"}))
	complete := Values{ReceiptDate: day("2026-11-03"), Amount: ptr("23.50"), Currency: "MYR", ReceiptType: ptr("grab_fare"),
		ClaimType: ptr("travel"), Purpose: ptr("Client meeting at Acme")}
	assert.Empty(t, Missing(complete))
	complete.Purpose = ptr("   ")
	complete.Amount = ptr("0.00")
	assert.Equal(t, []string{FieldAmount, FieldPurpose}, Missing(complete), "a blank purpose or a zero amount is missing")
	assert.Empty(t, Missing(Values{ReceiptDate: day("2026-11-03"), Amount: ptr("5.00"), ReceiptType: ptr("food"),
		ClaimType: ptr("other"), Purpose: ptr("Lunch")}), "the merchant and the client or candidate are optional")
}

func TestReceiptDates(t *testing.T) {
	today := *day("2026-11-19")
	assert.True(t, ValidReceiptDate(*day("2026-11-19"), today))
	assert.True(t, ValidReceiptDate(*day("2026-11-20"), today), "a day ahead: time zones")
	assert.False(t, ValidReceiptDate(*day("2026-11-21"), today))
	assert.False(t, ValidReceiptDate(*day("1999-12-31"), today))
}

func TestTextIsTrimmedAndLimited(t *testing.T) {
	got, ok := CleanText(ptr("  Lunch   with\tclient "), 20)
	assert.True(t, ok)
	assert.Equal(t, "Lunch with client", *got)
	got, ok = CleanText(ptr("   "), 20)
	assert.True(t, ok)
	assert.Nil(t, got)
	_, ok = CleanText(ptr(strings.Repeat("é", 21)), 20)
	assert.False(t, ok, "counted in characters")
}

func TestLinksFitTheirKind(t *testing.T) {
	id := strings.Repeat("a", 26)
	assert.True(t, ValidLink(LinkCompany, "comp_"+id))
	assert.True(t, ValidLink(LinkContact, "cont_"+id))
	assert.True(t, ValidLink(LinkCandidate, "cand_"+id))
	assert.False(t, ValidLink(LinkCompany, "cand_"+id), "a candidate is not a company")
	assert.False(t, ValidLink("job", "job_"+id))
	assert.False(t, ValidLink(LinkContact, "cont_short"))
}

func TestReadingFillsOnlyEmptyFields(t *testing.T) {
	read := Read{ReceiptDate: day("2026-11-03"), Amount: ptr("23.50"), Currency: ptr("SGD"), Merchant: ptr("Grab"), ReceiptType: ptr("grab_fare")}
	got, filled := Prefill(Values{Currency: "MYR"}, read, true)
	assert.Equal(t, []string{FieldReceiptDate, FieldAmount, FieldCurrency, FieldMerchant, FieldReceiptType}, filled)
	assert.Equal(t, "SGD", got.Currency)
	assert.Equal(t, "23.50", *got.Amount)

	mine := Values{Amount: ptr("20.00"), Currency: "MYR", Merchant: ptr("Grab ride to Acme")}
	got, filled = Prefill(mine, read, false)
	assert.Equal(t, "20.00", *got.Amount, "the person's amount stands")
	assert.Equal(t, "Grab ride to Acme", *got.Merchant)
	assert.Equal(t, "MYR", got.Currency, "a claim already edited keeps its currency")
	assert.Equal(t, []string{FieldReceiptDate, FieldReceiptType}, filled)

	got, filled = Prefill(Values{Currency: "MYR"}, Read{}, true)
	assert.Empty(t, filled, "nothing read, nothing filled")
	assert.Equal(t, "MYR", got.Currency)
}

func TestSuggestionsAreUnmarkedOnceChanged(t *testing.T) {
	from := Marked(nil, []string{FieldAmount, FieldMerchant})
	assert.Equal(t, []string{FieldAmount, FieldMerchant}, from)
	assert.Equal(t, []string{FieldMerchant}, Unmark(from, []string{FieldAmount, FieldPurpose}))
	assert.Equal(t, []string{FieldAmount, FieldMerchant, FieldReceiptType}, Marked(from, []string{FieldReceiptType, FieldAmount}))
	assert.NotNil(t, Unmark(nil, nil), "an empty list, never null")
}

func TestClaimTypeSuggestedByReceiptType(t *testing.T) {
	assert.Equal(t, "travel", ClaimTypeFor("grab_fare"))
	assert.Equal(t, "travel", ClaimTypeFor("parking_toll"))
	assert.Equal(t, "office", ClaimTypeFor("phone"))
	assert.Empty(t, ClaimTypeFor("food"), "a client's lunch or the team's: the claimant says")
	assert.Empty(t, ClaimTypeFor("hotel_custom"))
}

func TestDecisions(t *testing.T) {
	cases := []struct {
		decision, status, to string
		ok                   bool
	}{
		{Approve, Submitted, Approved, true},
		{Reject, Submitted, Rejected, true},
		{Pay, Approved, Paid, true},
		{Approve, Draft, "", false},
		{Pay, Submitted, "", false},
		{Reject, Paid, "", false},
		{Approve, Approved, "", false},
	}
	for _, c := range cases {
		to, ok := Decide(c.decision, c.status)
		assert.Equal(t, c.ok, ok, "%s %s", c.decision, c.status)
		assert.Equal(t, c.to, to)
	}
	assert.True(t, Editable(Draft))
	assert.True(t, Editable(Rejected))
	assert.False(t, Editable(Submitted))
	assert.False(t, Editable(Paid))
}

func TestAReceiptStillReadingTooLongShowsAsNotRead(t *testing.T) {
	uploaded := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	status, problem := OcrShown(OcrReading, nil, uploaded, uploaded.Add(9*time.Minute))
	assert.Equal(t, OcrReading, status)
	assert.Nil(t, problem)
	status, problem = OcrShown(OcrReading, nil, uploaded, uploaded.Add(11*time.Minute))
	assert.Equal(t, OcrFailed, status)
	assert.Equal(t, tooLongReading, *problem)
	read := "No text could be read."
	status, problem = OcrShown(OcrFailed, &read, uploaded, uploaded.Add(time.Hour))
	assert.Equal(t, OcrFailed, status)
	assert.Equal(t, &read, problem)
	status, _ = OcrShown(OcrDone, nil, uploaded, uploaded.Add(time.Hour))
	assert.Equal(t, OcrDone, status)
}
