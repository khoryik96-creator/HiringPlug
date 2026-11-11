package salarycalc

import (
	"regexp"
	"strings"
)

var wordingWords = regexp.MustCompile(`[A-Za-z]+`)
var perfBonus = regexp.MustCompile(`(?i)\bperf\.?\s+bonus\b`)
var epfRate = regexp.MustCompile(`(?i)\bepf\s*(\d+(?:,\d{3})*(?:\.\d+)?)(\s*%)?(?: \(unit not specified\))?`)

// CleanWording tidies salary vocabulary for presentation only. It never changes
// amounts, recalculates pay, or rewrites the recruiter's stored wording.
func CleanWording(text string) string {
	text = perfBonus.ReplaceAllString(text, "performance bonus")
	text = epfRate.ReplaceAllStringFunc(text, func(value string) string {
		match := epfRate.FindStringSubmatch(value)
		if strings.TrimSpace(match[2]) == "%" {
			return match[1] + "% EPF"
		}
		return "EPF " + match[1] + " (unit not specified)"
	})
	return wordingWords.ReplaceAllStringFunc(text, func(word string) string {
		lower := strings.ToLower(word)
		if lower == "epf" || lower == "rm" || lower == "aws" {
			return strings.ToUpper(lower)
		}
		if lower == "allowances" {
			return lower
		}
		for _, known := range []string{"allowance", "performance", "contractual", "guaranteed", "bonus"} {
			if lower == known || (lower[0] == known[0] && nearWord(lower, known)) {
				return known
			}
		}
		return word
	})
}
