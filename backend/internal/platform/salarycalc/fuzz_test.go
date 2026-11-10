package salarycalc

import (
	"math"
	"strings"
	"testing"
)

func FuzzSalaryCalculation(f *testing.F) {
	for _, s := range [][2]string{{"16k + 400 allowance + AWS", "10% lateral"}, {"16k", "1000%"}, {"16,50", "-12000"}, {"", "22-25k"}, {"MYR 16k + USD 400 allowance", "SGD same"}, {"16000", strings.Repeat("9", 400) + "-" + strings.Repeat("9", 401) + "k"}} {
		f.Add(s[0], s[1])
	}
	for _, s := range [][2]string{{"9.6k + 13.5 month bonus + 3 months bonus", "14-18%"}, {"9k + no AWS", "10-12k or 15k"}, {"108k/yr", "120k/yr"}, {"300 allowance + 9k + AWS", "20% decrease"}} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, current, expected string) {
		if len(current) > 2000 || len(expected) > 2000 {
			t.Skip()
		}
		r := Calculate(current, expected, DefaultRules(), "MYR")
		for _, n := range []float64{r.Base, r.Allowances, r.Bonus, r.Monthly, r.AnnualMonths} {
			if math.IsInf(n, 0) || math.IsNaN(n) {
				t.Fatal("calculator returned a non-finite value")
			}
		}
		if !r.ReviewRequired {
			if !validMoney(r.Monthly) {
				t.Fatal("unreviewed fixed salary outside the supported range")
			}
			if r.ExpectedMin != nil && !validMoney(*r.ExpectedMin) {
				t.Fatal("invalid expected salary")
			}
			if r.ExpectedMax != nil && (r.ExpectedMin == nil || *r.ExpectedMax < *r.ExpectedMin || !validMoney(*r.ExpectedMax)) {
				t.Fatal("invalid expected range")
			}
		}
	})
}
