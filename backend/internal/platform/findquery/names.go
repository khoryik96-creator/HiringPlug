package findquery

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MinNameScore is the least NameScore worth offering as a name alike.
const MinNameScore = 0.55

// Patronymic connectors, left out when comparing names: "Amirul bin Hakim"
// is "Amirul Hakim".
var connectors = map[string]bool{"bin": true, "binti": true, "binte": true, "bte": true, "bt": true, "ibn": true}

// fold is s in lower case with accents and punctuation out, in words.
func fold(s string) []string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

// NameTokens are the words a name is compared by; connectors are left out
// unless the name is nothing else.
func NameTokens(name string) []string {
	tokens := fold(name)
	var meaningful []string
	for _, t := range tokens {
		if !connectors[t] {
			meaningful = append(meaningful, t)
		}
	}
	if len(meaningful) == 0 {
		return tokens
	}
	return meaningful
}

// NameScore is how alike a name is to the one searched for, 0 to 1. Each
// word searched takes its best match among the name's words, so a name in
// another order scores almost as well (0.98 of it); a word that starts the
// other ("amir", "Amirul") scores high, since people type part of a first
// name; otherwise it is the share of letters not needing a change.
func NameScore(query, name string) float64 {
	q, c := NameTokens(query), NameTokens(name)
	if len(q) == 0 || len(c) == 0 {
		return 0
	}
	total := 0.0
	for _, qt := range q {
		best := 0.0
		for _, ct := range c {
			best = max(best, tokenScore(qt, ct))
			if best == 1 {
				break
			}
		}
		total += best
	}
	score := total / float64(len(q))
	if strings.Join(q, " ") != strings.Join(c, " ") {
		score *= 0.98
	}
	return score
}

func tokenScore(a, b string) float64 {
	if a == b {
		return 1
	}
	s, t := []rune(a), []rune(b)
	short, long := min(len(s), len(t)), max(len(s), len(t))
	if strings.HasPrefix(b, a) || strings.HasPrefix(a, b) {
		return 0.9*float64(short)/float64(long) + 0.1
	}
	return max(0, 1-float64(editDistance(s, t))/float64(long))
}

// editDistance is the Levenshtein distance between s and t.
func editDistance(s, t []rune) int {
	prev := make([]int, len(t)+1)
	cur := make([]int, len(t)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(s); i++ {
		cur[0] = i
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(t)]
}

// NameProbes are the words to look up names alike by, besides the whole
// name: each word of three letters or more, and the first letters of each
// (so a misspelt ending still finds them).
func NameProbes(query string) []string {
	var out []string
	add := func(s string) {
		if len([]rune(s)) >= 3 && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	tokens := NameTokens(query)
	for _, t := range tokens {
		add(t)
	}
	for _, t := range tokens {
		r := []rune(t)
		add(string(r[:min(4, len(r)-1)]))
	}
	return out
}

// CompanyKey is a company name folded for comparing: lower case, no
// accents, punctuation as spaces.
func CompanyKey(s string) string { return strings.Join(fold(s), " ") }

// CompanyWords are the words of the company searched for.
func CompanyWords(company string) []string { return fold(company) }

// WorksAt reports an employer that is the company searched for, word for
// word: "Nintex" is "Nintex Malaysia Sdn Bhd", not "Nintexify".
func WorksAt(employer, company string) bool {
	q := CompanyKey(company)
	return q != "" && strings.Contains(" "+CompanyKey(employer)+" ", " "+q+" ")
}
