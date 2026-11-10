package salarycalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecruiterDescriptions(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"16k monthly + 400 allowance + shift allowance + 13.5th month bonus + 3 months performance + 16% epf", 18400},
		{"16k + 400 allowance + 13.5 fixed bonus", 18400},
		{"16k + 400 allowance + fixed bonus 14", 19067},
		{"16k + 400 allowance + 15 months contractual bonus", 20400},
		{"16k + 400 allowance + 1.5 mths AWS", 18400},
		{"16k + 400 allowance + 13.5 contracutal bouns", 18400},
		{"16k + AWS", 17333},
		{"16k + 400 allowance + 1.5 months contrctual bonuss", 18400},
		{"16k + 400 allowance + 13.5 months gauranteed bonus", 18400}, //nolint:misspell // Deliberately tests recruiter typing mistakes.
		{"16k + 400 allowance + AWS1.5", 18400},
		{"16k per month + 400 allowance + fixed bonu 13.5", 18400},
		{"16k + annual wage supplement", 17333},
		{"16k + 2 months contractual bonus", 18667},
		{"16k + 13th bonus", 17333},
		{"16k + 3 months fixed bonus", 20000},
		{"16k + 14 months experience + 3 months performance bonus", 16000},
		{"16k + up to 3 months contractual bonus", 16000},
		{"16k + 800 shift allowance", 16000},
		{"16k + 800 fixed shift allowance", 16800},
		{"6000 + 300 allowance + 420 + 3 bonus + 31% bonus + 15% EPF + 14 month bonus", 7720},
		{"SGD12,000 + 500 housing", 12500},
		{"5.000,50 + 400 allowance", 5401},
		{"14.4 lakhs per annum", 120000},
	} {
		t.Run(tc.raw, func(t *testing.T) { assert.Equal(t, tc.want, Calculate(tc.raw, "", DefaultRules()).Monthly) })
	}
}

func TestExpectedSalary(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		lo, hi float64
	}{
		{"10% lateral", 20240, 0}, {"lateral", 18400, 0}, {"same as current", 18400, 0},
		{"22k", 22000, 0}, {"22-25k", 22000, 25000}, {"10-15% increment", 20240, 21160},
		{"MYR 25k or 20% increment", 25000, 0}, {"25k lateral", 25000, 0},
		{"15k + 20% bonus", 15000, 0}, {"-10%", 16560, 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			r := Calculate("16k + 400 allowance + 13.5th bonus", tc.raw, DefaultRules())
			require.NotNil(t, r.ExpectedMin)
			assert.Equal(t, tc.lo, *r.ExpectedMin)
			if tc.hi == 0 {
				assert.Nil(t, r.ExpectedMax)
			} else {
				require.NotNil(t, r.ExpectedMax)
				assert.Equal(t, tc.hi, *r.ExpectedMax)
			}
		})
	}
	for _, raw := range []string{"600%", "-100%", "20-10%", "25k-22k", "negotiable"} {
		r := Calculate("16k", raw, DefaultRules())
		assert.Nil(t, r.ExpectedMin, raw)
		assert.NotEmpty(t, r.Warnings, raw)
	}
}

func TestCompanyFormulaAndReview(t *testing.T) {
	text := "16k + 400 allowance + 13.5th month bonus"
	rules := DefaultRules()
	rules.Bonus = false
	assert.Equal(t, 16400.0, Calculate(text, "", rules).Monthly)
	rules = DefaultRules()
	rules.Allowances = false
	assert.Equal(t, 18000.0, Calculate(text, "", rules).Monthly)
	rules = DefaultRules()
	rules.BonusOnAllowances = true
	assert.Equal(t, 18450.0, Calculate(text, "", rules).Monthly)
	rules = DefaultRules()
	rules.Cents = true
	assert.Equal(t, 17333.33, Calculate("16k + AWS", "", rules).Monthly)
	for _, raw := range []string{"16k + fixed bonus", "16k + 13th bonus + 15th bonus", "daily rate 1.6k", "salary unknown"} {
		assert.NotEmpty(t, Calculate(raw, "", DefaultRules()).Warnings, raw)
	}
	r := Calculate("16k + 1.5 onus", "", DefaultRules())
	assert.Equal(t, 16000.0, r.Monthly)
	assert.NotEmpty(t, r.Warnings, "unrecognised bonus wording must not turn into a fixed allowance")
}

func TestCurrencyOnlyFromExplicitSalaryText(t *testing.T) {
	for raw, want := range map[string]string{
		"SGD12,000": "SGD", "270kUSD": "USD", "SGD 5000 / MYR 15000": "SGD",
		"MYR 8000 (SGD 2400)": "MYR", "PHP7.4": "", "20k": "", "14.4 lakhs": "INR",
		"RM 16k": "MYR", "GBP 8000": "GBP", "£8k": "GBP", "S$ 5000": "SGD", "20kSGD": "SGD",
	} {
		assert.Equal(t, want, Currency(raw), raw)
	}
	assert.Equal(t, 16000000.0, Calculate("Basic IDR 15.5mil nett, 16mil gross, THR bonus", "", DefaultRules()).Monthly)
}
