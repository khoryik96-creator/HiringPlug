// Package salarycalc ports Kano's deterministic salary rules to Go.
// It never calls a model or writes candidate data. Uncertain inputs need review.
package salarycalc

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Rules are the company's fixed-cash calculation choices.
type Rules struct {
	Allowances        bool `json:"allowances"`
	Bonus             bool `json:"bonus"`
	BonusOnAllowances bool `json:"bonusOnAllowances"`
	Cents             bool `json:"cents"`
}

// DefaultRules matches Kano: guaranteed bonus on basic pay, then allowances.
func DefaultRules() Rules { return Rules{Allowances: true, Bonus: true} }

// Result contains reviewed figures and their interpretation, never raw AI output.
type Result struct {
	Base, Allowances, Bonus, Monthly float64
	ExpectedMin, ExpectedMax         *float64
	AnnualMonths                     float64
	Excluded, Warnings               []string
}

var (
	numberRE       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(crores?|cr|lakhs?|lacs?|lpa|million|mil|k|l|m)?\b`)
	groupRE        = regexp.MustCompile(`(\d),(\d{3})(\D|$)`)
	decimalGroupRE = regexp.MustCompile(`\b\d{1,3}(?:\.\d{3})+,\d+\b`)
	multipleDotRE  = regexp.MustCompile(`\b\d{1,3}(?:\.\d{3}){2,}\b`)
	variableRE     = regexp.MustCompile(`(?i)rsu|stock|option|incentive|commission|variable|performance|epf|socso|eis|claim|reimburse|discretionary|up\s+to|not\s+guaranteed|non[- ]guaranteed`)
	conditionalRE  = regexp.MustCompile(`(?i)shift|overtime|attendance|conditional`)
	tenureRE       = regexp.MustCompile(`(?i)\b(experience|tenure|service)\b`)
	guaranteedRE   = regexp.MustCompile(`(?i)\b(aws|fixed\s+bonus|guaranteed\s+bonus|contractual\s+bonus|thr|festive|hari\s*raya)\b`)
	ordinalRE      = regexp.MustCompile(`(?i)\b(1[3-6](?:\.5)?)\s*(th|st|nd|rd)\s*(months?\s*)?(bonus|pay|salary|months?)\b`)
	monthBonusRE   = regexp.MustCompile(`(?i)\b(1[3-6](?:\.5)?)\s*months?\s*(bonus|guaranteed|salary|pay|package)\b`)
	bonusRE        = regexp.MustCompile(`(?i)bonus|^\s*\d+(?:\.\d+)?\s*months?\s*$`)
	sameRE         = regexp.MustCompile(`(?i)\b(same|similar|maintain|match|follow|lateral|horizontal)\b|no\s+(increment|increase|change)|as\s+current|current\s+(salary|package)`)
	percentRE      = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)\s*(?:%|percent)?\s*(?:-|–|~|to)\s*(-?\d+(?:\.\d+)?)\s*(?:%|percent)|(-?\d+(?:\.\d+)?)\s*(?:%|percent)`)
	rangeRE        = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(k|million|mil|lakhs?|lacs?|lpa|l|crores?|cr|m)?\s*(?:-|–|to)\s*(\d+(?:\.\d+)?)\s*(k|million|mil|lakhs?|lacs?|lpa|l|crores?|cr|m)?\b`)
	actualRE       = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(k|million|mil|lakh|lac|lpa|crore|cr)\b|\b\d{4,}\b`)
	annualRE       = regexp.MustCompile(`(?i)\bannual|per\s+(annum|annual|year)|yearly|\bpa\b|p\.a\.|\blpa\b`)
	nonMonthlyRE   = regexp.MustCompile(`(?i)hourly|per\s+hour|p\.h\.|daily|per\s+day|p\.d\.|weekly|per\s+week|p\.w\.`)
)

func clean(raw string) string {
	raw = correctSpelling(raw)
	raw = regexp.MustCompile(`([A-Za-z])(\d)`).ReplaceAllString(raw, "$1 $2")
	raw = regexp.MustCompile(`(?i)\bbonu\b`).ReplaceAllString(raw, "bonus")
	t := decimalGroupRE.ReplaceAllStringFunc(raw, func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".") })
	t = multipleDotRE.ReplaceAllStringFunc(t, func(s string) string { return strings.ReplaceAll(s, ".", "") })
	for groupRE.MatchString(t) {
		t = groupRE.ReplaceAllString(t, "$1$2$3")
	}
	//nolint:misspell // Deliberate recruiter input misspellings.
	return strings.NewReplacer("fixedbonus", "fixed bonus", "contractualbonus", "contractual bonus", "bouns", "bonus", "bnous", "bonus", "bonous", "bonus", "bonsu", "bonus", "contracutal", "contractual", "contracual", "contractual", "contractul", "contractual", "guarenteed", "guaranteed", "guarnateed", "guaranteed", "garanteed", "guaranteed", "fixd", "fixed", "mths", "months", "mth", "month", "mnths", "months", "mnth", "month", "monhts", "months", "annual wage supplement", "aws").Replace(strings.ToLower(t))
}

func magnitude(s string) float64 {
	switch s {
	case "k":
		return 1000
	case "m", "mil", "million":
		return 1000000
	case "l", "lpa", "lakh", "lakhs", "lac", "lacs":
		return 100000
	case "cr", "crore", "crores":
		return 10000000
	default:
		return 1
	}
}

func amount(s string) (float64, bool) {
	m := numberRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	n *= magnitude(m[2])
	return n, err == nil && !math.IsInf(n, 0) && n >= 0 && n <= 999999999999
}

func rounded(n float64, cents bool) float64 {
	if cents {
		return math.Round((n+1e-9)*100) / 100
	}
	return math.Round(n)
}

// Calculate computes fixed monthly cash from a complete description. Unlike Kano's
// regex fallback it does not truncate the current line to 80 characters.
func Calculate(current, expected string, rules Rules) Result {
	out := Result{AnnualMonths: 12, Excluded: []string{}, Warnings: []string{}}
	baseFound := false
	parts := strings.FieldsFunc(clean(current), func(r rune) bool { return r == '+' || r == ';' || r == ',' || r == '\n' })
	// Gross/nett alternatives are not additive allowances.
	pick := -1
	million := regexp.MustCompile(`(?i)\d\s*(mil|million)\b`)
	for i, part := range parts {
		if million.MatchString(part) && !bonusRE.MatchString(part) && !variableRE.MatchString(part) {
			if pick < 0 || strings.Contains(part, "gross") {
				pick = i
			}
			if strings.Contains(part, "gross") {
				break
			}
		}
	}
	if pick >= 0 {
		filtered := []string{parts[pick]}
		for i, part := range parts {
			if i != pick && !million.MatchString(part) {
				filtered = append(filtered, part)
			}
		}
		parts = filtered
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if regexp.MustCompile(`(^|\s)-\s*\d`).MatchString(part) {
			out.Warnings = append(out.Warnings, "A negative salary component needs review.")
			continue
		}
		if variableRE.MatchString(part) || tenureRE.MatchString(part) || strings.Contains(part, "%") {
			out.Excluded = append(out.Excluded, part)
			continue
		}
		if conditionalRE.MatchString(part) && !strings.Contains(part, "fixed") && !strings.Contains(part, "guaranteed") {
			out.Excluded = append(out.Excluded, part)
			out.Warnings = append(out.Warnings, "Conditional allowances excluded unless confirmed fixed.")
			continue
		}
		months, isBonus, known := bonusMonths(part)
		if isBonus {
			if known {
				if out.AnnualMonths != 12 && out.AnnualMonths != months {
					out.Warnings = append(out.Warnings, "Conflicting guaranteed bonus totals.")
				}
				out.AnnualMonths = months
			} else {
				out.Warnings = append(out.Warnings, "Enter the guaranteed bonus amount or months.")
			}
			continue
		}
		if bonusRE.MatchString(part) {
			if strings.Contains(part, "bonus") {
				out.Warnings = append(out.Warnings, "Enter the guaranteed bonus months, or mark this bonus as variable.")
			}
			out.Excluded = append(out.Excluded, part)
			continue
		}
		n, ok := amount(part)
		if !ok {
			out.Warnings = append(out.Warnings, "A salary component could not be read.")
			continue
		}
		if !baseFound {
			if nonMonthlyRE.MatchString(part) {
				out.Warnings = append(out.Warnings, "Working hours or days are needed to convert this rate to monthly pay.")
				return out
			}
			if annualRE.MatchString(part) {
				n /= 12
			}
			out.Base, baseFound = n, true
		} else if rules.Allowances {
			// An unfamiliar misspelled bonus must not silently become an allowance.
			remaining := numberRE.ReplaceAllString(part, "")
			knownAllowance := regexp.MustCompile(`(?i)allowance|fixed|guaranteed|transport|housing|meal|phone|mobile|internet|parking|petrol|fuel|travel|living`).MatchString(part)
			if regexp.MustCompile(`[a-z]`).MatchString(remaining) && !knownAllowance && Currency(part) == "" {
				out.Warnings = append(out.Warnings, "A salary component could not be read. Mark fixed allowances or variable bonuses explicitly.")
				continue
			}
			out.Allowances += n
		}
	}
	if !baseFound {
		out.Warnings = append(out.Warnings, "Enter a basic salary.")
		return out
	}
	// IDR gross already includes THR in Kano's input convention.
	if strings.Contains(clean(current), "gross") && regexp.MustCompile(`(?i)\bidr\b|rupiah|\brp\s*\d`).MatchString(current) {
		out.AnnualMonths = 12
	}
	if rules.Bonus {
		basis := out.Base
		if rules.BonusOnAllowances {
			basis += out.Allowances
		}
		out.Bonus = basis * (out.AnnualMonths - 12) / 12
	}
	out.Monthly = rounded(out.Base+out.Allowances+out.Bonus, rules.Cents)
	out.Bonus = rounded(out.Bonus, rules.Cents)
	calculateExpected(&out, clean(expected), rules)
	return out
}

func bonusMonths(s string) (float64, bool, bool) {
	for _, re := range []*regexp.Regexp{ordinalRE, monthBonusRE} {
		if m := re.FindStringSubmatch(s); m != nil {
			n, _ := strconv.ParseFloat(m[1], 64)
			return n, true, true
		}
	}
	if !guaranteedRE.MatchString(s) {
		return 0, false, false
	}
	if n, ok := amount(s); ok {
		if n >= 12 && n <= 18 {
			return n, true, true
		}
		if n >= 0 && n <= 6 {
			return 12 + n, true, true
		}
		return 0, true, false
	}
	if strings.Contains(s, "aws") || strings.Contains(s, "thr") || strings.Contains(s, "festive") || strings.Contains(s, "hari raya") {
		return 13, true, true
	}
	return 0, true, false
}

func calculateExpected(out *Result, s string, rules Rules) {
	if strings.TrimSpace(s) == "" {
		return
	}
	if nonMonthlyRE.MatchString(s) {
		out.Warnings = append(out.Warnings, "Expected rate needs working-time information before monthly conversion.")
		return
	}
	set := func(lo, hi float64) {
		lo = rounded(lo, rules.Cents)
		hi = rounded(hi, rules.Cents)
		out.ExpectedMin = &lo
		if hi != lo {
			out.ExpectedMax = &hi
		}
	}
	if actualRE.MatchString(s) || numberRE.MatchString(percentRE.ReplaceAllString(s, "")) || !strings.Contains(s, "%") && !strings.Contains(s, "percent") && !sameRE.MatchString(s) {
		if m := rangeRE.FindStringSubmatch(s); m != nil {
			a, _ := strconv.ParseFloat(m[1], 64)
			b, _ := strconv.ParseFloat(m[3], 64)
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
				out.Warnings = append(out.Warnings, "Expected salary range runs backwards.")
				return
			}
			set(a, b)
			return
		}
		if n, ok := amount(s); ok {
			if annualRE.MatchString(s) {
				n /= 12
			}
			set(n, n)
			return
		}
	}
	if m := percentRE.FindStringSubmatch(s); m != nil {
		lo, hi := 0.0, 0.0
		if m[3] != "" {
			lo, _ = strconv.ParseFloat(m[3], 64)
			hi = lo
		} else {
			lo, _ = strconv.ParseFloat(m[1], 64)
			hi, _ = strconv.ParseFloat(m[2], 64)
		}
		if lo <= -100 || hi > 500 || lo > hi || lo > 500 || hi <= -100 {
			out.Warnings = append(out.Warnings, "Expected increase must be above -100% and at most 500%.")
			return
		}
		set(out.Monthly*(1+lo/100), out.Monthly*(1+hi/100))
		return
	}
	if sameRE.MatchString(s) {
		set(out.Monthly, out.Monthly)
		return
	}
	out.Warnings = append(out.Warnings, "Expected salary needs review.")
}
