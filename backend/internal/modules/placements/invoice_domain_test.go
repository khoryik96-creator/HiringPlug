package placements

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceAmountsAndMoney(t *testing.T) {
	got, err := InvoiceAmounts("21600.00", "8")
	require.NoError(t, err)
	assert.Equal(t, InvoiceTotals{Subtotal: "21600.00", Tax: "1728.00", Total: "23328.00"}, got)
	got, err = InvoiceAmounts("1000.05", "6")
	require.NoError(t, err)
	assert.Equal(t, "60.00", got.Tax, "60.003 rounds to the cent")
	assert.Equal(t, "1060.05", got.Total)
	got, _ = InvoiceAmounts("500", "0")
	assert.Equal(t, InvoiceTotals{Subtotal: "500.00", Tax: "0.00", Total: "500.00"}, got)

	assert.Equal(t, "MYR 21,600.00", MoneyText("MYR", "21600.00"))
	assert.Equal(t, "SGD 1,234,567.50", MoneyText("SGD", "1234567.5"))
	assert.Equal(t, "MYR 999.00", MoneyText("MYR", "999"))
	assert.Equal(t, "120000.00", annualSalary(Salary{Amount: "10000.00", Currency: "MYR", Period: "month"}))
}

func TestInvoiceNumbersDatesAndSending(t *testing.T) {
	assert.Equal(t, "INV-0042", InvoiceNumber("INV-", 42))
	assert.Equal(t, "HYP-12345", InvoiceNumber("HYP-", 12345))

	start := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-12-31", DueOn(start, 30, nil).Format(time.DateOnly))
	set := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, set, DueOn(start, 30, &set), "a due date someone set stays")

	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	before := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 12, 1, 9, 0, 0, 0, kl), SendsAt(start, 9, kl, before), "9 am there on the start date")
	late := time.Date(2026, 12, 3, 3, 0, 0, 0, time.UTC)
	assert.Equal(t, late, SendsAt(start, 9, kl, late), "at once when that is past")
}

func TestInvoiceTemplates(t *testing.T) {
	values := map[string]string{"candidate.name": "Siti Aisyah", "invoice.total": "MYR 23,328.00"}
	assert.Equal(t, "Siti Aisyah: MYR 23,328.00 {{agency.nickname}}",
		FillInvoice("{{candidate.name}}: {{ invoice.total }} {{agency.nickname}}", values))

	s := DefaultInvoiceSettings("Hyppies")
	_, errs := NormalizeInvoiceSettings(s)
	assert.Empty(t, errs, "the defaults are valid")

	s.EmailBody, s.TaxPercent, s.Issuer = "Hi {{contact.nickname}}", "101", "  "
	_, errs = NormalizeInvoiceSettings(s)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	assert.Equal(t, map[string]bool{"emailBody": true, "taxPercent": true, "issuer": true}, fields)

	doc := InvoiceDoc(DefaultInvoiceSettings("Hyppies"), map[string]string{"invoice.number": "INV-0001"}, []string{"Maju", "Ken"}, false)
	for _, b := range doc {
		assert.NotContains(t, b.Cells, "SST 0%", "no tax line without tax")
	}
}
