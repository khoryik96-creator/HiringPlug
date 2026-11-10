package salarycalc

import (
	"regexp"
	"strconv"
	"strings"
)

func calculateExpected(out *Result, text string, rules Rules) {
	if strings.TrimSpace(text) == "" {
		return
	}
	parts := strings.FieldsFunc(groupedNumbers(text), func(r rune) bool { return r == '+' || r == ';' || r == ',' || r == '\n' })
	primary := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if bonusRE.MatchString(clean(part)) || variableRE.MatchString(clean(part)) {
			continue
		}
		primary = append(primary, part)
	}
	primary = preferGross(primary)
	if len(primary) == 0 {
		review(out, "Expected salary needs review.")
		return
	}
	raw := strings.TrimSpace(primary[0])
	out.ExpectedCurrency = currencyHint(raw, true)
	s := clean(raw)
	if nonMonthlyRE.MatchString(s) {
		review(out, "Expected rate needs working-time information before monthly conversion.")
		return
	}
	set := func(lo, hi float64) {
		lo, hi = rounded(lo, rules.Cents), rounded(hi, rules.Cents)
		if !validMoney(lo) || !validMoney(hi) || lo > hi {
			review(out, "Expected salary is outside the supported range.")
			return
		}
		out.ExpectedMin = &lo
		if hi != lo {
			out.ExpectedMax = &hi
		}
	}
	// Removing percentages first prevents 1000% from being read as salary 1000.
	literal := percentRE.ReplaceAllString(s, "")
	if numberRE.MatchString(literal) {
		if negativeMoney(literal) {
			review(out, "Expected salary cannot be negative.")
			return
		}
		if m := rangeRE.FindStringSubmatch(literal); m != nil {
			a, errA := strconv.ParseFloat(m[1], 64)
			b, errB := strconv.ParseFloat(m[3], 64)
			if errA != nil || errB != nil {
				review(out, "Expected salary is outside the supported range.")
				return
			}
			left, right := m[2], m[4]
			if a < 1000 && b < 1000 {
				if left == "" {
					left = right
				}
				if right == "" {
					right = left
				}
			}
			a *= magnitude(left)
			b *= magnitude(right)
			if annualRE.MatchString(s) {
				a /= 12
				b /= 12
			}
			if a > b {
				review(out, "Expected salary range runs backwards.")
				return
			}
			set(a, b)
			return
		}
		if len(numberRE.FindAllString(literal, -1)) != 1 {
			review(out, "Enter one expected amount or an explicit range.")
			return
		}
		if n, ok := amount(literal); ok {
			if annualRE.MatchString(s) {
				n /= 12
			}
			set(n, n)
			return
		}
		review(out, "Expected salary is outside the supported range.")
		return
	}
	matches := percentRE.FindAllStringSubmatch(s, -1)
	if len(matches) > 0 {
		if len(matches) != 1 {
			review(out, "Enter one percentage or a percentage range.")
			return
		}
		m := matches[0]
		lo, hi := 0.0, 0.0
		if m[3] != "" {
			lo, _ = strconv.ParseFloat(m[3], 64)
			hi = lo
		} else {
			lo, _ = strconv.ParseFloat(m[1], 64)
			hi, _ = strconv.ParseFloat(m[2], 64)
		}
		if lo <= -100 || hi > 500 || lo > hi || lo > 500 || hi <= -100 {
			review(out, "Expected increase must be above -100% and at most 500%.")
			return
		}
		out.relativeExpected = true
		set(out.Monthly*(1+lo/100), out.Monthly*(1+hi/100))
		return
	}
	if sameRE.MatchString(s) {
		out.relativeExpected = true
		set(out.Monthly, out.Monthly)
		return
	}
	// Unknown words must not be turned into a guessed numeric salary.
	if regexp.MustCompile(`\S`).MatchString(s) {
		review(out, "Expected salary needs review.")
	}
}
