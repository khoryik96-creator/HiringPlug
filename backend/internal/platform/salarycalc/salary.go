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
	Base, Allowances, Bonus, Monthly             float64
	ExpectedMin, ExpectedMax                     *float64
	AnnualMonths                                 float64
	Excluded, Warnings                           []string
	HasCurrent, ReviewRequired, relativeExpected bool
	bonusCurrency                                string
	CurrentCurrency, ExpectedCurrency            string
}

var (
	numberRE       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(crores?|cr|lakhs?|lacs?|lpa|million|mil|k|l|m)?\b`)
	groupedRE      = regexp.MustCompile(`\d+(?:,\d+)+`)
	westernGroupRE = regexp.MustCompile(`^\d{1,3}(?:,\d{3})+$`)
	indianGroupRE  = regexp.MustCompile(`^\d{1,3}(?:,\d{2})*,\d{3}$`)
	decimalGroupRE = regexp.MustCompile(`\b\d{1,3}(?:\.\d{3})+,\d+\b`)
	multipleDotRE  = regexp.MustCompile(`\b\d{1,3}(?:\.\d{3}){2,}\b`)
	variableRE     = regexp.MustCompile(`(?i)rsu|stock|option|incentive|commission|variable|performance|epf|socso|eis|claim|reimburse|discretionary|up\s+to|not\s+(?:guaranteed|fixed)|non[- ](?:guaranteed|fixed)|\b(?:no|not|without)\s+(?:\d+(?:\.\d+)?(?:th|st|nd|rd)?\s*(?:months?\s*)?)?(?:aws|thr|(?:fixed\s+|guaranteed\s+|contractual\s+)?bonus)\b`)
	conditionalRE  = regexp.MustCompile(`(?i)shift|overtime|attendance|conditional`)
	allowanceRE    = regexp.MustCompile(`(?i)allowance|transport|housing|meal|phone|mobile|internet|parking|petrol|fuel|travel|living`)
	tenureRE       = regexp.MustCompile(`(?i)\b(experience|tenure|service)\b`)
	guaranteedRE   = regexp.MustCompile(`(?i)\b(aws|(?:fixed|guaranteed|contractual)\s+(?:(?:annual|yearly)\s+)?bonus|thr|festive|hari\s*raya)\b`)
	ordinalRE      = regexp.MustCompile(`(?i)\b(1[3-6](?:\.5)?)\s*(th|st|nd|rd)\s*(months?\s*)?(bonus|pay|salary|months?)\b`)
	monthBonusRE   = regexp.MustCompile(`(?i)\b(1[3-6](?:\.5)?)\s*months?\s*(bonus|guaranteed|salary|pay|package)\b`)
	bonusRE        = regexp.MustCompile(`(?i)bonus|^\s*\d+(?:\.\d+)?\s*months?\s*$`)
	bonusNumberRE  = regexp.MustCompile(`\d+(?:\.\d+)?`)
	sameRE         = regexp.MustCompile(`(?i)\b(same|similar|maintain|match|follow|lateral|horizontal)\b|no\s+(increment|increase|change)|as\s+current|current\s+(salary|package)`)
	percentRE      = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)\s*(?:%|percent)?\s*(?:-|–|~|to)\s*(-?\d+(?:\.\d+)?)\s*(?:%|percent)|(-?\d+(?:\.\d+)?)\s*(?:%|percent)`)
	rangeRE        = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(k|million|mil|lakhs?|lacs?|lpa|l|crores?|cr|m)?\s*(?:-|–|~|to)\s*(\d+(?:\.\d+)?)\s*(k|million|mil|lakhs?|lacs?|lpa|l|crores?|cr|m)?\b`)
	annualRE       = regexp.MustCompile(`(?i)\bannual|per\s+(annum|annual|year)|yearly|\bpa\b|p\.a\.|\blpa\b|/\s*(?:yr|year|annum|annual|pa)\b`)
	nonMonthlyRE   = regexp.MustCompile(`(?i)hourly|per\s+hour|p\.h\.|daily|per\s+day|p\.d\.|weekly|per\s+week|p\.w\.|fortnight|bi[- ]weekly|/\s*(?:h|hr|hour|d|day|w|wk|week)\b`)
)

func clean(raw string) string {
	raw = strings.NewReplacer("−", "-", "—", "–", "％", "%").Replace(raw)
	raw = regexp.MustCompile(`(^|[^0-9])\.(\d)`).ReplaceAllString(raw, "${1}0.${2}")
	raw = percentWords(raw)
	raw = componentPlus(raw)
	raw = regexp.MustCompile(`([A-Za-z])(\d)`).ReplaceAllString(raw, "$1 $2")
	raw = regexp.MustCompile(`(?i)\b(lpa|lakhs?|lacs?|crores?|cr)\s+(\d+(?:\.\d+)?)`).ReplaceAllString(raw, "$2 $1")
	raw = regexp.MustCompile(`(?i)(\d[kml]?)(BND|KHR|IDR|LAK|MYR|MMK|PHP|SGD|THB|VND|CNY|HKD|AUD|GBP|EUR|USD|INR)\b`).ReplaceAllString(raw, "$1 $2")
	raw = regexp.MustCompile(`(?i)\bbonu\b`).ReplaceAllString(raw, "bonus")
	t := groupedNumbers(raw)
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
func Calculate(current, expected string, rules Rules, currency ...string) (out Result) {
	out = Result{AnnualMonths: 12, Excluded: []string{}, Warnings: []string{}}
	defer func() { finish(&out) }()
	fallback := ""
	if len(currency) > 0 {
		fallback = strings.ToUpper(strings.TrimSpace(currency[0]))
	}
	rawCurrency := Currency(current)
	baseFound := false
	parts := strings.FieldsFunc(clean(current), func(r rune) bool { return r == '+' || r == ';' || r == ',' || r == '\n' })
	parts = preferBase(preferGross(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if negativeMoney(part) {
			review(&out, "A negative salary component needs review.")
			continue
		}
		if variableRE.MatchString(part) || tenureRE.MatchString(part) || strings.Contains(part, "%") {
			if strings.Contains(part, "%") && guaranteedRE.MatchString(part) && !variableRE.MatchString(part) {
				review(&out, "Enter the guaranteed bonus as extra months or total annual months; its percentage basis needs review.")
			}
			out.Excluded = append(out.Excluded, part)
			continue
		}
		if conditionalRE.MatchString(part) && (!strings.Contains(part, "fixed") && !strings.Contains(part, "guaranteed") || regexp.MustCompile(`not\s+fixed|non[- ]fixed|not\s+guaranteed`).MatchString(part)) {
			out.Excluded = append(out.Excluded, part)
			inform(&out, "Conditional allowances excluded unless confirmed fixed.")
			continue
		}
		if rangeRE.MatchString(part) {
			review(&out, "Choose one fixed salary or guaranteed bonus figure, not a range.")
			continue
		}
		months, isBonus, known := bonusMonths(part)
		if isBonus {
			if !rules.Bonus {
				out.Excluded = append(out.Excluded, part)
				continue
			}
			if len(bonusNumberRE.FindAllString(part, -1)) > 1 {
				review(&out, "Enter one guaranteed bonus figure without additional alternatives.")
				continue
			}
			if known {
				if c := Currency(part); c != "" {
					if out.bonusCurrency != "" && out.bonusCurrency != c {
						review(&out, "Guaranteed bonuses use different currencies.")
					}
					out.bonusCurrency = c
				}
				if out.AnnualMonths != 12 && out.AnnualMonths != months {
					review(&out, "Conflicting guaranteed bonus totals.")
				}
				out.AnnualMonths = months
			} else {
				review(&out, "Enter the guaranteed bonus as extra months or total annual months.")
			}
			continue
		}
		if bonusRE.MatchString(part) {
			if rules.Bonus && strings.Contains(part, "bonus") {
				inform(&out, "Bonuses without guaranteed terms are excluded.")
			}
			out.Excluded = append(out.Excluded, part)
			continue
		}
		if len(numberRE.FindAllString(part, -1)) > 1 {
			review(&out, "Enter one fixed amount or separate package components with +.")
			continue
		}
		n, ok := amount(part)
		if !ok {
			review(&out, "A salary component could not be read.")
			continue
		}
		if !baseFound {
			if allowanceRE.MatchString(part) {
				review(&out, "Enter the basic salary separately from fixed allowances.")
				continue
			}
			if nonMonthlyRE.MatchString(part) {
				review(&out, "Working hours or days are needed to convert this rate to monthly pay.")
				return out
			}
			if annualRE.MatchString(part) {
				n /= 12
			}
			out.Base, baseFound, out.HasCurrent = n, true, true
			out.CurrentCurrency = Currency(part)
			if rawCurrency == "" || out.CurrentCurrency == "" {
				out.CurrentCurrency = fallback
			}
		} else if rules.Allowances {
			if c := Currency(part); c != "" && (out.CurrentCurrency == "" || c != out.CurrentCurrency) {
				review(&out, "Allowance currency differs from the basic salary; enter a converted fixed amount.")
				continue
			}
			if nonMonthlyRE.MatchString(part) {
				review(&out, "Allowance rate needs working-time information before monthly conversion.")
				continue
			}
			if annualRE.MatchString(part) {
				n /= 12
			}
			// An unfamiliar misspelled bonus must not silently become an allowance.
			remaining := numberRE.ReplaceAllString(part, "")
			knownAllowance := regexp.MustCompile(`(?i)allowance|fixed|guaranteed|transport|housing|meal|phone|mobile|internet|parking|petrol|fuel|travel|living`).MatchString(part)
			if regexp.MustCompile(`[a-z]`).MatchString(remaining) && !knownAllowance && Currency(part) == "" {
				review(&out, "A salary component could not be read. Mark fixed allowances or variable bonuses explicitly.")
				continue
			}
			out.Allowances += n
		}
	}
	if !baseFound {
		if strings.TrimSpace(current) != "" {
			review(&out, "Enter a basic salary.")
		}
		calculateExpected(&out, expected, rules)
		return out
	}
	// IDR gross already includes THR in Kano's input convention.
	if strings.Contains(clean(current), "gross") && out.CurrentCurrency == "IDR" {
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
	calculateExpected(&out, expected, rules)
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

func groupedNumbers(raw string) string {
	raw = strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(raw)
	raw = regexp.MustCompile(`\b\d{1,3}(?: \d{3})+(?:\.\d+)?\b`).ReplaceAllStringFunc(raw, func(s string) string { return strings.ReplaceAll(s, " ", "") })
	t := decimalGroupRE.ReplaceAllStringFunc(raw, func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".") })
	t = multipleDotRE.ReplaceAllStringFunc(t, func(s string) string { return strings.ReplaceAll(s, ".", "") })
	t = groupedRE.ReplaceAllStringFunc(t, func(token string) string {
		if westernGroupRE.MatchString(token) || indianGroupRE.MatchString(token) {
			return strings.ReplaceAll(token, ",", "")
		}
		return " [unreadable number] "
	})
	return t
}

func componentPlus(raw string) string {
	return regexp.MustCompile(`(?i)\bplus\b`).ReplaceAllString(raw, "+")
}

// Preserve glued currency-code boundaries while accepting common percentage wording.
func percentWords(raw string) string {
	//nolint:misspell // Common truncated percent word in recruiter input.
	return regexp.MustCompile(`(?i)(\d)\s*(?:percent(?:age)?|per\s+cent|pct|percen)\b`).ReplaceAllString(correctSpelling(raw), "$1%")
}
