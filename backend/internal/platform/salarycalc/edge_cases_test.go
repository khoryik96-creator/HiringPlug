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
