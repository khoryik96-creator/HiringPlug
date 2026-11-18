package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSalaryFiguresAreTakenOut(t *testing.T) {
	for in, want := range map[string]string{
		"Expected salary: RM 5,000 negotiable":                      "Expected [salary removed]",
		"Accountant, 2019-2023. Last drawn pay RM8.5k":              "Accountant, 2019-2023. [salary removed]",
		"Gaji semasa: 4500":                                         "[salary removed]",
		"Current package / CTC: MYR 120,000 a year":                 "Current package / [salary removed]",
		"Managed compensation and benefits for 300 staff":           "Managed compensation and benefits for 300 staff",
		"Processed salary for staff, cut errors by half since 2019": "Processed salary for staff, cut errors by half since 2019",
		"Expected salary: 2000":                                     "Expected [salary removed]",
		"Saved RM 1.2 million on audit fees":                        "Saved RM 1.2 million on audit fees",
	} {
		assert.Equal(t, want, WithoutSalary(in), in)
	}
}

func TestWithoutMoneyTakesOutEveryAmount(t *testing.T) {
	got := WithoutMoney("She expects RM 8,765 a month, or $5k, or 9k, or MYR9500, or 12,500; 5 years since 2019.")
	for _, leak := range []string{"8,765", "5k", "9k", "9500", "12,500"} {
		if strings.Contains(got, leak) {
			t.Errorf("%q still has %q", got, leak)
		}
	}
	for _, kept := range []string{"5 years", "2019"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q lost %q", got, kept)
		}
	}
}
