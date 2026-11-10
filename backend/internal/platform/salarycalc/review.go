package salarycalc

import (
	"math"
	"regexp"
	"slices"
	"strings"
)

const maxMoney = 999999999999.99

func validMoney(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= maxMoney
}

func review(out *Result, message string) {
	out.ReviewRequired = true
	inform(out, message)
}

func inform(out *Result, message string) {
	if !slices.Contains(out.Warnings, message) {
		out.Warnings = append(out.Warnings, message)
	}
}

func finish(out *Result) {
	if out.bonusCurrency != "" && out.CurrentCurrency != "" && out.bonusCurrency != out.CurrentCurrency {
		review(out, "Guaranteed bonus currency differs from the basic salary.")
	}
	if out.relativeExpected {
		if !out.HasCurrent {
			review(out, "A current fixed salary is needed for a percentage or lateral expectation.")
			out.ExpectedMin, out.ExpectedMax = nil, nil
		} else if out.ExpectedCurrency != "" && out.ExpectedCurrency != out.CurrentCurrency {
			review(out, "A percentage or lateral expectation cannot change currency without a converted amount.")
			out.ExpectedMin, out.ExpectedMax = nil, nil
		} else {
			out.ExpectedCurrency = out.CurrentCurrency
		}
	}
	for _, n := range []float64{out.Base, out.Allowances, out.Bonus, out.Monthly} {
		if !validMoney(n) {
			review(out, "The calculated salary is outside the supported range.")
		}
	}

}

// Keep gross/nett alternatives separate from additive package components.
func preferGross(parts []string) []string {
	million := regexp.MustCompile(`(?i)\d\s*(mil|million|m)\b`)
	alternative := func(s string) bool {
		return million.MatchString(s) && !bonusRE.MatchString(s) && !variableRE.MatchString(s)
	}
	first, pick := -1, -1
	for i, part := range parts {
		if !alternative(part) {
			continue
		}
		if first < 0 {
			first = i
			pick = i
		}
		if strings.Contains(part, "gross") {
			pick = i
			break
		}
	}
	if pick < 0 {
		return parts
	}
	chosen := parts[pick]
	if Currency(chosen) == "" {
		if c := Currency(parts[first]); c != "" {
			chosen = c + " " + chosen
		}
	}
	if annualRE.MatchString(parts[first]) && !annualRE.MatchString(chosen) && !nonMonthlyRE.MatchString(chosen) {
		chosen = "annual " + chosen
	}
	out := []string{chosen}
	for i, part := range parts {
		if i != pick && !alternative(part) {
			out = append(out, part)
		}
	}
	return out
}
