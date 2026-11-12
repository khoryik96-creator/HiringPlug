package salary

import (
	"math/big"
	"regexp"
	"strings"
)

func calculateExpected(out *Result, text string, rules Rules) {
	if strings.TrimSpace(text) == "" {
		return
	}
	parts := strings.FieldsFunc(componentPlus(groupedNumbers(text)), func(r rune) bool { return r == '+' || r == ';' || r == ',' || r == '\n' })
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
	if len(primary) != 1 {
		review(out, "Enter one expected salary or percentage range; combine fixed components into a reviewed monthly figure.")
		return
	}
	raw := strings.TrimSpace(primary[0])
	out.ExpectedCurrency = currencyHint(percentWords(raw), true)
	s := clean(raw)
	if nonMonthlyRE.MatchString(s) {
		review(out, "Expected rate needs working-time information before monthly conversion.")
		return
	}
	set := func(lo, hi *big.Rat) {
		lo, hi = rounded(lo, rules.Cents), rounded(hi, rules.Cents)
		if !validMoney(lo) || !validMoney(hi) || lo.Cmp(hi) > 0 {
			review(out, "Expected salary is outside the supported range.")
			return
		}
		low := moneyOf(lo)
		out.ExpectedMin = &low
		if hi.Cmp(lo) != 0 {
			high := moneyOf(hi)
			out.ExpectedMax = &high
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
			if len(numberRE.FindAllString(literal, -1)) != 2 {
				review(out, "Enter one expected range without additional alternative amounts.")
				return
			}
			a, okA := decimal(m[1])
			b, okB := decimal(m[3])
			if !okA || !okB {
				review(out, "Expected salary is outside the supported range.")
				return
			}
			left, right := m[2], m[4]
			thousand := big.NewRat(1000, 1)
			if a.Cmp(thousand) < 0 && b.Cmp(thousand) < 0 {
				if left == "" {
					left = right
				}
				if right == "" {
					right = left
				}
			}
			a, b = mul(a, magnitude(left)), mul(b, magnitude(right))
			if annualRE.MatchString(s) {
				a, b = quo(a, twelve), quo(b, twelve)
			}
			if a.Cmp(b) > 0 {
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
				n = quo(n, twelve)
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
		var lo, hi *big.Rat
		if m[3] != "" {
			lo, _ = decimal(m[3])
			hi = lo
		} else {
			lo, _ = decimal(m[1])
			hi, _ = decimal(m[2])
		}
		minus100, plus500 := big.NewRat(-100, 1), big.NewRat(500, 1)
		if lo == nil || hi == nil || lo.Cmp(minus100) <= 0 || hi.Cmp(plus500) > 0 || lo.Cmp(hi) > 0 || hi.Cmp(minus100) <= 0 {
			review(out, "Expected increase must be above -100% and at most 500%.")
			return
		}
		if (lo.Sign() > 0 || hi.Sign() > 0) && decreaseRE.MatchString(s) {
			review(out, "Use a negative percentage for an expected salary decrease.")
			return
		}
		out.relativeExpected = true
		raise := func(p *big.Rat) *big.Rat { return mul(out.Monthly.rat(), add(big.NewRat(1, 1), quo(p, hundred))) }
		set(raise(lo), raise(hi))
		return
	}
	if sameRE.MatchString(s) {
		out.relativeExpected = true
		set(out.Monthly.rat(), out.Monthly.rat())
		return
	}
	// Unknown words must not be turned into a guessed numeric salary.
	if anyTextRE.MatchString(s) {
		review(out, "Expected salary needs review.")
	}
}

var (
	decreaseRE = regexp.MustCompile(`\b(decrease|decrement|reduction|reduce|less|lower|cut)\b`)
	anyTextRE  = regexp.MustCompile(`\S`)
)
