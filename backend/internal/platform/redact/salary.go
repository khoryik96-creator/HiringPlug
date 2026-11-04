package redact

import (
	"regexp"
	"strings"
)

var (
	salaryWord = regexp.MustCompile(`(?i)\b(?:salary|salaries|gaji|remuneration|compensation|ctc|take[- ]home|(?:expected|current|last[- ]drawn|basic)\s+pay|pay\s*:)`)
	amount     = regexp.MustCompile(`(?i)(?:rm|myr|sgd|usd|idr|s\$|\$)\s?\d|\d[\d,.]*\s?k\b|\d{1,3}(?:,\d{3})+|\d{4,}`)
	year       = regexp.MustCompile(`^(?:19|20)\d\d$`)
)

// WithoutSalary takes salary figures out of free text (a CV, a job
// description) before it goes to the AI: on each line, from a salary word to
// the end of the line, when an amount follows it ("Expected salary: RM
// 5,000"). Duties such as "compensation and benefits for 300 staff" are
// kept: no money follows.
func WithoutSalary(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		for _, loc := range salaryWord.FindAllStringIndex(line, -1) {
			if hasAmount(line[loc[1]:]) {
				lines[i] = line[:loc[0]] + "[salary removed]"
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// hasAmount reports whether text (after a salary word) holds money. A
// number like a year counts only right after the word ("Salary: 2000"), so
// "salary for staff since 2019" keeps its year.
func hasAmount(text string) bool {
	for _, loc := range amount.FindAllStringIndex(text, -1) {
		if !year.MatchString(text[loc[0]:loc[1]]) || loc[0] <= 3 {
			return true
		}
	}
	return false
}
