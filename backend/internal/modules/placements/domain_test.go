package placements

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func TestNormalize(t *testing.T) {
	today := day("2026-11-03")
	base := Details{StartDate: day("2026-12-01"), GuaranteeDays: 90}

	d, errs, problem := Normalize(base, today)
	assert.Empty(t, errs)
	assert.Empty(t, problem)
	assert.Equal(t, Active, d.Status)
	assert.Equal(t, NotInvoiced, d.InvoiceStatus)
	assert.Equal(t, day("2027-03-01"), d.GuaranteeEnds())

	t.Run("a fall-through is within the guarantee", func(t *testing.T) {
		in := base
		in.Status = FellThrough
		_, errs, _ := Normalize(in, today)
		require.Len(t, errs, 1)
		assert.Equal(t, "leftOn", errs[0].Field)
		in.LeftOn = ptr(day("2027-01-15"))
		_, errs, problem := Normalize(in, today)
		assert.Empty(t, errs)
		assert.Empty(t, problem)
		in.LeftOn = ptr(day("2027-03-02"))
		_, _, problem = Normalize(in, today)
		assert.Contains(t, problem, "ended on 1 March 2027")
		in.LeftOn = ptr(day("2026-11-30"))
		_, errs, _ = Normalize(in, today)
		assert.Equal(t, "leftOn", errs[0].Field)
	})

	t.Run("back to active forgets the day they left", func(t *testing.T) {
		in := base
		in.LeftOn = ptr(day("2027-01-15"))
		d, _, _ := Normalize(in, today)
		assert.Nil(t, d.LeftOn)
	})

	t.Run("invoice dates", func(t *testing.T) {
		in := base
		in.InvoiceStatus, in.InvoiceNumber = Invoiced, ptr("  INV-0042 ")
		d, _, _ := Normalize(in, today)
		assert.Equal(t, today, *d.InvoicedOn, "invoiced today unless said otherwise")
		assert.Equal(t, "INV-0042", *d.InvoiceNumber)
		in.InvoiceStatus, in.InvoicedOn = Paid, ptr(day("2026-12-05"))
		d, _, _ = Normalize(in, today)
		assert.Equal(t, today, *d.PaidOn)
		in.PaidOn = ptr(day("2026-12-01"))
		_, errs, _ := Normalize(in, today)
		assert.Equal(t, "paidOn", errs[0].Field)
		in.InvoiceStatus = NotInvoiced
		d, _, _ = Normalize(in, today)
		assert.Nil(t, d.InvoiceNumber)
		assert.Nil(t, d.InvoicedOn)
		assert.Nil(t, d.PaidOn)
	})
}

func TestMoney(t *testing.T) {
	m, errs := NormalizeMoney(Money{Salary: &Salary{Amount: "8500.00", Currency: "MYR", Period: "month"}, FeePercent: ptr("20")})
	assert.Empty(t, errs)
	assert.Equal(t, "20400.00", *m.FeeAmount, "20% of 12 months")
	assert.Equal(t, "MYR", *m.FeeCurrency)

	m, _ = NormalizeMoney(Money{Salary: &Salary{Amount: "100000", Currency: "SGD", Period: "year"}, FeePercent: ptr("17.5")})
	assert.Equal(t, "17500.00", *m.FeeAmount)
	m, _ = NormalizeMoney(Money{Salary: &Salary{Amount: "1000.03", Currency: "MYR", Period: "month"}, FeePercent: ptr("12.5")})
	assert.Equal(t, "1500.05", *m.FeeAmount, "1500.045: to the cent, half a cent rounded up")

	_, errs = NormalizeMoney(Money{Salary: &Salary{Amount: "50", Currency: "MYR", Period: "hour"}, FeePercent: ptr("20")})
	assert.Equal(t, "money.feePercent", errs[0].Field, "no yearly salary from an hourly rate")
	_, errs = NormalizeMoney(Money{FeePercent: ptr("20")})
	assert.Equal(t, "money.feePercent", errs[0].Field)
	_, errs = NormalizeMoney(Money{FeePercent: ptr("120"), Salary: &Salary{Amount: "1", Currency: "MYR", Period: "year"}})
	assert.Equal(t, "money.feePercent", errs[0].Field)
	_, errs = NormalizeMoney(Money{FeeAmount: ptr("5000")})
	assert.Equal(t, "money.feeAmount", errs[0].Field)
	m, errs = NormalizeMoney(Money{FeeAmount: ptr("5000"), FeeCurrency: ptr("MYR")})
	assert.Empty(t, errs)
	assert.Nil(t, m.FeePercent)
}

func TestTotals(t *testing.T) {
	totals := Totals([]FeeLine{
		{Status: Active, InvoiceStatus: Paid, Amount: "20400.00", Currency: "MYR"},
		{Status: Active, InvoiceStatus: Invoiced, Amount: "10000.50", Currency: "MYR"},
		{Status: Active, InvoiceStatus: NotInvoiced, Amount: "8000", Currency: "SGD"},
		{Status: FellThrough, InvoiceStatus: Paid, Amount: "9999", Currency: "MYR"},
	})
	assert.Equal(t, []FeeTotal{
		{Currency: "MYR", Total: "30400.50", NotInvoiced: "0.00", Invoiced: "10000.50", Paid: "20400.00"},
		{Currency: "SGD", Total: "8000.00", NotInvoiced: "8000.00", Invoiced: "0.00", Paid: "0.00"},
	}, totals, "fallen-through placements do not count")
}
