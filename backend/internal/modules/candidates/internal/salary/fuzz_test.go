package salary

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
		if math.IsInf(r.AnnualMonths, 0) || math.IsNaN(r.AnnualMonths) {
			t.Fatal("calculator returned non-finite months")
		}
		if !r.ReviewRequired {
			if !validMoney(r.Monthly.rat()) {
				t.Fatal("unreviewed fixed salary outside the supported range")
			}
			if r.ExpectedMin != nil && !validMoney(r.ExpectedMin.rat()) {
				t.Fatal("invalid expected salary")
			}
			if r.ExpectedMax != nil && (r.ExpectedMin == nil || r.ExpectedMax.rat().Cmp(r.ExpectedMin.rat()) < 0 || !validMoney(r.ExpectedMax.rat())) {
				t.Fatal("invalid expected range")
			}
		}
	})
}
