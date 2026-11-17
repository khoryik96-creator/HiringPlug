package jobs

// What a blind ad keeps out besides the client's name: the details that let
// a reader work out who the client is (its headcount, how many countries or
// offices, "the leading/largest", when it was founded, where it is listed,
// its revenue). Rules, so a blind ad is safe without the AI rewrite; the AI
// rewrite (Rewrite with AI) words it better.

import (
	"regexp"
	"strings"
)

type softening struct {
	re   *regexp.Regexp
	with string
}

const countWord = `(?:\d[\d,.]*\+?|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|fifteen|twenty|thirty|forty|fifty|a dozen|dozens of|hundreds of|thousands of)`

var softenings = []softening{
	// "2,000 staff", "over 5,000 employees", "a team of 300 people"
	{regexp.MustCompile(`(?i)\b(?:a (?:team|workforce|headcount) of\s+)?(?:(?:over|more than|about|around|some|nearly|close to|almost|approximately|~)\s*)?` + countWord + `\s*(?:k\s+)?(?:staff|employees|people|professionals|team members|headcount|colleagues|employees worldwide)\b`), "a sizeable team"},
	// "across 5 countries", "in 12 markets"
	{regexp.MustCompile(`(?i)\b(?:across|in|over)\s+(?:(?:over|more than)\s+)?` + countWord + `\s+(?:countries|markets|nations|territories|cities|regions)\b`), "across the region"},
	// "200 branches", "over 50 offices"
	{regexp.MustCompile(`(?i)\b(?:(?:over|more than|about|around)\s+)?` + countWord + `\s+(?:branches|offices|outlets|stores|plants|factories|sites|locations|hospitals|clinics|hotels)\b`), "a network of sites"},
	// "founded in 1965", "established since 1998", "since 1970"
	{regexp.MustCompile(`(?i)\b(?:founded|established|incorporated|set up|started)\s+(?:in|since)?\s*(?:19|20)\d\d\b|\bsince\s+(?:19|20)\d\d\b`), "well established"},
	// "listed on Bursa Malaysia", "a SGX-listed", "NYSE listed"
	{regexp.MustCompile(`(?i)\blisted on (?:the )?(?:main market of )?(?:bursa malaysia|bursa|sgx|singapore exchange|nyse|nasdaq|hkex|hong kong stock exchange|idx|set|pse|asx|lse|london stock exchange)\b`), "publicly listed"},
	{regexp.MustCompile(`(?i)\b(?:bursa(?: malaysia)?|sgx|nyse|nasdaq|hkex|asx|idx|lse)[- ]listed\b`), "publicly listed"},
	// "revenue of RM 2.5 billion", "USD 300 million in assets"
	{regexp.MustCompile(`(?i)\b(?:rm|usd|us\$|s\$|sgd|myr|idr|\$|€|£)\s?[\d.,]+\s?(?:billion|bn|million|mil|m|trillion|tn)\b`), "a significant amount"},
	// "the leading", "the largest", "number one", "No. 1", "#1", "market-leading", "the biggest"
	{regexp.MustCompile(`(?i)\b(?:number one|no\.\s?1)\b|#1\b`), "an established"},
	{regexp.MustCompile(`(?i)\b(?:the\s+)?(?:largest|biggest|leading|market-leading|foremost|premier|most established)\b`), "established"},
}

// a and an before what follows ("a established" -> "an established").
var article = regexp.MustCompile(`(?i)\b(a|an)\s+([a-z]+)`)

func fixArticle(m string) string {
	parts := article.FindStringSubmatch(m)
	art, word := parts[1], parts[2]
	vowel := strings.ContainsRune("aeiouAEIOU", rune(word[0]))
	want := "a"
	if vowel {
		want = "an"
	}
	if art[0] == 'A' {
		want = strings.ToUpper(want[:1]) + want[1:]
	}
	return want + " " + word
}

// Soften takes the identifying details out of an ad's text (before the
// client's name is replaced, so the description a person chose is kept as
// written). A replacement at the start of a sentence starts with a capital.
func Soften(text string) string {
	for _, s := range softenings {
		var b strings.Builder
		last := 0
		for _, m := range s.re.FindAllStringIndex(text, -1) {
			b.WriteString(text[last:m[0]])
			with := s.with
			if sentenceStart(text[:m[0]]) {
				with = capitalize(with)
			}
			b.WriteString(with)
			last = m[1]
		}
		b.WriteString(text[last:])
		text = b.String()
	}
	text = regexp.MustCompile(`(?i)\b(an?) (an established|a sizeable)\b`).ReplaceAllString(text, "$2")
	text = regexp.MustCompile(`(?i)\b(the) (an established)\b`).ReplaceAllString(text, "$2")
	text = article.ReplaceAllStringFunc(text, fixArticle)
	return regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(text, " ")
}

// ourClientIs words a sentence that starts with the client's description and
// says what it is ("A regional investment bank is a Malaysian investment bank")
// as "Our client is a Malaysian investment bank".
func ourClientIs(text, description string) string {
	d := strings.TrimSpace(description)
	if d == "" {
		return text
	}
	re := regexp.MustCompile(`(?m)(^|[.!?]\s+)` + regexp.QuoteMeta(capitalize(d)) + `\s+(is|are)\s+(an?|one of)\b`)
	return re.ReplaceAllString(text, "${1}Our client ${2} ${3}")
}
