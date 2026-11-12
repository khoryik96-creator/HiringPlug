package salary

import (
	"regexp"
	"strings"
)

var salaryWords = regexp.MustCompile(`[a-zA-Z]{4,12}`)

// Correct only near-matches to known salary vocabulary. Unknown terms remain
// visible; this is not a general spellchecker or a guess at an unknown package.
func correctSpelling(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "bonuses", "bonus")
	text = strings.ReplaceAll(text, "fixed annual bonus", "fixed bonus")
	return salaryWords.ReplaceAllStringFunc(text, func(word string) string {
		for _, known := range []string{"bonus", "contractual", "guaranteed", "allowance", "months", "monthly", "annual", "annum", "yearly", "daily", "weekly", "hourly"} {
			if word == known {
				return word
			}
			if word[0] == known[0] && nearWord(word, known) {
				return known
			}
		}
		return word
	})
}

func nearWord(a, b string) bool {
	if len(a) == len(b) {
		positions := []int{}
		for i := range a {
			if a[i] != b[i] {
				positions = append(positions, i)
			}
		}
		if len(positions) == 1 {
			return true
		}
		if len(positions) == 2 {
			i, j := positions[0], positions[1]
			return j == i+1 && a[i] == b[j] && a[j] == b[i]
		}
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b)-len(a) != 1 {
		return false
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		if i != j {
			return false
		}
		j++
	}
	return true
}
