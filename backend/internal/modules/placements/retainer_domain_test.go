package placements

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetainer(t *testing.T) {
	today := day("2026-11-12")
	r, errs := NormalizeRetainer(Retainer{Currency: "MYR", SignedOn: ptr(day("2026-11-01")), Stages: []RetainerStage{
		{Label: " Engagement ", Amount: "15000", DueKind: DueAfterSigning, DueDays: ptr(0), InvoiceStatus: Paid},
		{Label: "Shortlist", Amount: "15000.5", DueKind: DueOnDate, DueOn: ptr(day("2026-12-15")), DueDays: ptr(3)},
		{Label: "Completion", Amount: "5000", DueKind: DueAfterStart, DueDays: ptr(14), InvoiceNumber: ptr("x")},
	}}, today)
	require.NoError(t, errs.Err())
	assert.Equal(t, "Engagement", r.Stages[0].Label)
	assert.Equal(t, today, *r.Stages[0].PaidOn, "paid today unless said otherwise")
	assert.Equal(t, "15000.50", r.Stages[1].Amount)
	assert.Nil(t, r.Stages[1].DueDays, "a date has no days")
	assert.Nil(t, r.Stages[2].InvoiceNumber, "not invoiced: no number")
	assert.Equal(t, "35000.50", r.Total())

	assert.Equal(t, day("2026-11-01"), *r.DueDay(r.Stages[0], nil))
	assert.Equal(t, day("2026-12-15"), *r.DueDay(r.Stages[1], nil))
	assert.Nil(t, r.DueDay(r.Stages[2], nil), "after the start: not known until placed")
	assert.Equal(t, day("2027-01-15"), *r.DueDay(r.Stages[2], ptr(day("2027-01-01"))))

	balance, ok := r.Balance("45000.00", "MYR")
	assert.True(t, ok)
	assert.Equal(t, "9999.50", balance, "the retainer is taken off the fee")
	balance, _ = r.Balance("20000.00", "MYR")
	assert.Equal(t, "0.00", balance, "never below nothing")
	_, ok = r.Balance("45000.00", "SGD")
	assert.False(t, ok, "another currency: nothing taken off")

	_, errs = NormalizeRetainer(Retainer{Currency: "myr", Stages: []RetainerStage{
		{Label: "", Amount: "0", DueKind: DueOnDate},
		{Label: "B", Amount: "1", DueKind: DueAfterStart, DueDays: ptr(900)},
		{Label: "C", Amount: "1", DueKind: "later"},
		{Label: "D", Amount: "1", DueKind: DueOnDate, DueOn: ptr(today), InvoiceStatus: Paid, InvoicedOn: ptr(today), PaidOn: ptr(day("2026-11-01"))},
	}}, today)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, f := range []string{"currency", "stages.0.label", "stages.0.amount", "stages.0.dueOn", "stages.1.dueDays", "stages.2.dueKind", "stages.3.paidOn"} {
		assert.True(t, fields[f], f)
	}
	empty, errs := NormalizeRetainer(Retainer{}, today)
	assert.NoError(t, errs.Err(), "no stages: no retainer, no currency needed")
	assert.Equal(t, "0.00", empty.Total())
}
