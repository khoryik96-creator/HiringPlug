package salarycalc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedSalaryReviewFindings(t *testing.T) {
	for _, raw := range []string{"1000%", "900-1000%", "-12000", "−20k", strings.Repeat("9", 400) + "-" + strings.Repeat("9", 401) + "k"} {
		t.Run(raw[:min(len(raw), 35)], func(t *testing.T) {
			r := Calculate("16k", raw, DefaultRules())
			assert.Nil(t, r.ExpectedMin)
			assert.NotEmpty(t, r.Warnings)
		})
	}
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"15k + 20% annual bonus", 15000},
		{"15k + 5k annual performance bonus", 15000},
		{"IDR 15mil nett, 16mil gross", 16000000},
		{"IDR 15.5m nett, 16m gross", 16000000},
		{"−10% lateral", 14400},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			r := Calculate("16k", tc.raw, DefaultRules())
			require.NotNil(t, r.ExpectedMin)
			assert.Equal(t, tc.want, *r.ExpectedMin)
		})
	}
}

func TestCurrentSalaryReviewFindings(t *testing.T) {
	//nolint:misspell // Recruiter typing mistakes are intentional test input.
	assert.Equal(t, 16200.0, Calculate("16k + 2400 anual fixed allowance", "", DefaultRules()).Monthly)
	assert.Equal(t, 16200.0, Calculate("16k + 2400 annual fixed allowance", "", DefaultRules()).Monthly)
	assert.Equal(t, 16000000.0, Calculate("IDR 15.5m nett, 16m gross", "", DefaultRules()).Monthly)
	assert.Equal(t, 16000.0, Calculate("16k + 800 not fixed shift allowance", "", DefaultRules()).Monthly)
	for _, raw := range []string{"MYR 16k + USD 400 fixed allowance", "16k + 600 daily fixed allowance", "MYR 16k + SGD 1.5 months contractual bonus", "16k-18k", "16k + 1.5-2 months contractual bonus"} {
		assert.NotEmpty(t, Calculate(raw, "", DefaultRules()).Warnings, raw)
	}
	rules := DefaultRules()
	rules.Bonus = false
	assert.Empty(t, Calculate("16k + fixed bonus", "", rules).Warnings)
}

func TestNumericGroupingAndCurrencyReview(t *testing.T) {
	for raw, want := range map[string]float64{".5k": 500, "MYR 1,00,000": 100000, "LPA18": 150000, "16k + 1,500 allowance": 17500} {
		r := Calculate(raw, "", DefaultRules())
		assert.Equal(t, want, r.Monthly, raw)
		assert.False(t, r.ReviewRequired, raw)
	}
	for _, raw := range []string{"16,50", "16000,400 allowance", "MYR 16k + USD 400 fixed allowance", "16k + 1.5-2 months contractual bonus", "-16k + 400 allowance"} {
		assert.True(t, Calculate(raw, "", DefaultRules(), "MYR").ReviewRequired, raw)
	}
	for _, expected := range []string{"SGD 10% lateral", "USD10%", "SGD similar to current"} {
		r := Calculate("MYR 16k", expected, DefaultRules(), "MYR")
		assert.True(t, r.ReviewRequired, expected)
		assert.Nil(t, r.ExpectedMin, expected)
	}
	for _, raw := range []string{"1.2 cr", "18 LPA", "LPA18", "1.2cr", "18l"} {
		assert.Equal(t, "INR", Currency(raw), raw)
	}
	r := Calculate("SGD 16k", "maintain", DefaultRules(), "MYR")
	assert.Equal(t, "SGD", r.ExpectedCurrency)
	assert.False(t, r.ReviewRequired)
	r = Calculate("", "22k", DefaultRules(), "MYR")
	assert.False(t, r.HasCurrent)
	assert.False(t, r.ReviewRequired)
	require.NotNil(t, r.ExpectedMin)
	assert.Equal(t, 22000.0, *r.ExpectedMin)
	r = Calculate("", "20%", DefaultRules())
	assert.True(t, r.ReviewRequired)
	assert.Nil(t, r.ExpectedMin)
}

func TestExcludedBonusCannotSetBaseCurrency(t *testing.T) {
	assert.Equal(t, 22500.0, Calculate("annual 270kUSD base", "", DefaultRules()).Monthly)
	assert.True(t, Calculate("IDR 15.5mil nett / 16mil gross", "", DefaultRules()).ReviewRequired)
	r := Calculate("MYR 16k", "15k + USD 5k annual performance bonus", DefaultRules(), "MYR")
	require.NotNil(t, r.ExpectedMin)
	assert.Equal(t, 15000.0, *r.ExpectedMin)
	assert.Empty(t, r.ExpectedCurrency, "an excluded bonus cannot relabel unlabelled expected base pay")
	r = Calculate("16k + USD 5k performance bonus", "", DefaultRules())
	assert.Empty(t, r.CurrentCurrency, "an excluded bonus cannot supply missing base currency")
	r = Calculate("MYR 16k", "PHP7.4", DefaultRules(), "MYR")
	assert.NotEqual(t, "PHP", r.ExpectedCurrency, "normalisation must not turn a software-version-shaped token into pesos")
}

func TestNegativeSignBeforeCurrencyRequiresReview(t *testing.T) {
	for _, raw := range []string{"-MYR16k", "-USD 12000", "-RM.5k", "-S$8000", "−€12000"} {
		assert.True(t, Calculate(raw, "", DefaultRules()).ReviewRequired, raw)
		r := Calculate("16k", raw, DefaultRules())
		assert.True(t, r.ReviewRequired, raw)
		assert.Nil(t, r.ExpectedMin, raw)
	}
}

func TestAmbiguousExpectedClausesRequireReview(t *testing.T) {
	for _, expected := range []string{"10% + 15%", "10k + 500 allowance", "10-12k or 15k", "MYR 10-12k / SGD 3k", "20% decrease"} {
		r := Calculate("9k", expected, DefaultRules(), "MYR")
		assert.True(t, r.ReviewRequired, expected)
		assert.Nil(t, r.ExpectedMin, expected)
	}
}

func TestNegatedAndExplicitGuaranteedBonuses(t *testing.T) {
	for _, bonus := range []string{"no AWS", "not AWS", "without AWS", "no fixed bonus", "no 13th bonus", "without 3 months fixed bonus", "3 months not fixed bonus", "3 months non-fixed bonus"} {
		r := Calculate("9k + "+bonus, "", DefaultRules())
		assert.False(t, r.ReviewRequired, bonus)
		assert.Equal(t, 9000.0, r.Monthly, bonus)
	}
	for _, bonus := range []string{"3 months guaranteed annual bonus", "3 months contractual yearly bonus"} {
		r := Calculate("9k + "+bonus, "", DefaultRules())
		assert.False(t, r.ReviewRequired, bonus)
		assert.Equal(t, 11250.0, r.Monthly, bonus)
	}
	assert.True(t, Calculate("9k + 15% contractual bonus", "", DefaultRules()).ReviewRequired)
}

func TestSlashPayPeriods(t *testing.T) {
	for _, suffix := range []string{"/yr", "/year", "/annum"} {
		r := Calculate("108k"+suffix, "120k"+suffix, DefaultRules())
		assert.False(t, r.ReviewRequired, suffix)
		assert.Equal(t, 9000.0, r.Monthly, suffix)
		require.NotNil(t, r.ExpectedMin)
		assert.Equal(t, 10000.0, *r.ExpectedMin, suffix)
	}
	for _, suffix := range []string{"/hr", "/hour", "/day", "/wk", "/week", " fortnightly"} {
		assert.True(t, Calculate("900"+suffix, "", DefaultRules()).ReviewRequired, suffix)
		assert.True(t, Calculate("9k + 200 allowance"+suffix, "", DefaultRules()).ReviewRequired, suffix)
		r := Calculate("9k", "900"+suffix, DefaultRules())
		assert.True(t, r.ReviewRequired, suffix)
		assert.Nil(t, r.ExpectedMin, suffix)
	}
}

func TestNamedAllowanceCannotBecomeBasicPay(t *testing.T) {
	for _, current := range []string{"300 allowance + 9k + AWS", "300 allowance + 9k basic + AWS"} {
		r := Calculate(current, "", DefaultRules())
		assert.False(t, r.ReviewRequired, current)
		assert.Equal(t, 9000.0, r.Base, current)
		assert.Equal(t, 10050.0, r.Monthly, current)
	}
	for _, current := range []string{"300 allowance", "300 allowance + 500 housing", "-9k + 300 allowance"} {
		r := Calculate(current, "", DefaultRules())
		assert.True(t, r.ReviewRequired, current)
		assert.False(t, r.HasCurrent, current)
	}
}

func TestConflictingBonusFiguresInOneClause(t *testing.T) {
	for _, bonus := range []string{"13th bonus or 15th bonus", "1.5 months fixed bonus or 2 months contractual bonus", "MYR 13.5 month bonus / SGD 14 month bonus"} {
		assert.True(t, Calculate("9k + "+bonus, "", DefaultRules(), "MYR").ReviewRequired, bonus)
	}
}
