package salarycalc

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var displayMoney = regexp.MustCompile(`(?i)^\s*((?:RM|MYR|SGD|USD|HKD|AUD|GBP|EUR|INR|IDR|BND|THB|PHP|VND|CNY|S\$|US\$|HK\$|A\$|\$|£|€)\s*)?([0-9]+(?:,[0-9]{3})*(?:\.[0-9]+)?)\s*(k|mil|million|m)?\b(.*)$`)
var displayMonths = regexp.MustCompile(`(?i)^\s*(\d+(?:\.\d+)?)(?:th|st|nd|rd)?\s*(?:months?|mths?)\b`)
var displayPeriod = regexp.MustCompile(`(?i)\s*(?:per\s+month|monthly|/\s*(?:month|mth|mo))\b`)
var totalMonths = regexp.MustCompile(`(?i)^\s*(1[3-6](?:\.5)?)\s*months?\s+total\s*$`)

// FormatBreakdown expands recruiter shorthand for email presentation. Stored
// figures and original wording stay unchanged; months and percentages never
// become currency amounts and conditional pay is never labelled guaranteed.
func FormatBreakdown(raw, currency, period string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	parts := strings.FieldsFunc(componentPlus(raw), func(r rune) bool { return r == '+' || r == ';' || r == '\n' })
	for i, part := range parts {
		parts[i] = displayComponent(strings.TrimSpace(part), currency, i == 0)
	}
	result := strings.Join(parts, " + ")
	base := ""
	if len(parts) > 0 {
		base = strings.ToLower(parts[0])
	}
	if period == "month" && !annualRE.MatchString(base) && !nonMonthlyRE.MatchString(base) && !strings.Contains(strings.ToLower(result), "per month") {
		result += " per month"
	}
	return result
}

func displayComponent(raw, currency string, base bool) string {
	text := CleanWording(raw)
	normalized := clean(text)
	if tenureRE.MatchString(normalized) {
		return text
	}
	if variableRE.MatchString(normalized) && !allowanceRE.MatchString(normalized) {
		// Performance months/percentages are quantities, not money.
		if displayMonths.MatchString(text) || strings.Contains(text, "%") || strings.Contains(strings.ToLower(text), "epf") {
			return text
		}
	}
	if !variableRE.MatchString(normalized) && !conditionalRE.MatchString(normalized) {
		months, isBonus, known := bonusMonths(normalized)
		if match := totalMonths.FindStringSubmatch(normalized); match != nil {
			months, _ = strconv.ParseFloat(match[1], 64)
			isBonus, known = true, true
		}
		if isBonus && known && months >= 13 && months <= 18 {
			if count := displayMonths.FindStringSubmatch(text); count != nil {
				extra, _ := strconv.ParseFloat(count[1], 64)
				if extra < 12 {
					return count[1] + " months fixed/contractual bonus"
				}
			}
			return strconv.FormatFloat(months, 'f', -1, 64) + "th month fixed/contractual bonus"
		}
	}
	for _, group := range groupedRE.FindAllString(text, -1) {
		if !westernGroupRE.MatchString(group) && !indianGroupRE.MatchString(group) {
			return text + " (amount needs review)"
		}
	}
	match := displayMoney.FindStringSubmatch(groupedNumbers(text))
	if match == nil {
		return text
	}
	tail := strings.TrimSpace(match[4])
	// A count, percentage, range, or an unexplained small number is not money.
	if strings.HasPrefix(tail, "%") || displayMonths.MatchString(text) || strings.Contains(tail, "(unit not specified)") || strings.HasPrefix(tail, "-") || strings.HasPrefix(tail, "–") {
		return text
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(match[2], ",", ""), 64)
	n *= magnitude(strings.ToLower(match[3]))
	if err != nil || math.IsInf(n, 0) || n > 999999999999 {
		return text
	}
	if match[1] == "" && match[3] == "" && n < 100 && ((!base && tail == "") || bonusRE.MatchString(normalized)) {
		return text + " (component needs review)"
	}
	code := Currency(text)
	if code == "" {
		code = currency
	}
	if code == "" {
		return text
	}
	if code == "MYR" {
		code = "RM"
	}
	if allowanceRE.MatchString(normalized) && !variableRE.MatchString(normalized) && !conditionalRE.MatchString(normalized) && !strings.Contains(normalized, "fixed") && !strings.Contains(normalized, "guaranteed") {
		tail = "fixed " + tail
	}
	if base && !annualRE.MatchString(normalized) && !nonMonthlyRE.MatchString(normalized) {
		tail = strings.TrimSpace(displayPeriod.ReplaceAllString(tail, ""))
	}
	return strings.TrimSpace(code + " " + groupDisplayAmount(n) + " " + tail)
}

func groupDisplayAmount(n float64) string {
	value := strconv.FormatFloat(n, 'f', -1, 64)
	parts := strings.SplitN(value, ".", 2)
	whole := parts[0]
	for at := len(whole) - 3; at > 0; at -= 3 {
		whole = whole[:at] + "," + whole[at:]
	}
	if len(parts) > 1 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		whole += "." + fraction
	}
	return whole
}
