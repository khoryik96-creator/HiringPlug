package matching

// What possible candidates are looked for with: the job's distinctive words,
// weighted. The words every job description uses (experience, proven,
// within, skills, strong, ability, team...) say nothing about who fits and
// are left out; the job title's words, acronyms (ACCA, SAP, SQL) and the
// description's two-word terms ("statutory audit") count most. The database
// weighs each term again by how rare it is in the company's CVs.

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// SearchTerm is a word or a short phrase, with how much a CV having it counts.
type SearchTerm struct {
	Text   string
	Weight float32
}

const (
	maxSearchTerms = 30
	titleWeight    = 4
	titlePhrase    = 6
	acronymWeight  = 2.5
	phraseWeight   = 2.5
	wordWeight     = 1
	maxWordWeight  = 2.5
)

var wordRe = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}+#.&/-]*[\p{L}\p{N}+#]|[\p{L}\p{N}]`)

// generic are the words job descriptions use whatever the job: never searched for.
var generic = setOf(`a about above across after again against all also am an and any are as at be because been before being
below between both but by can could did do does doing down during each few for from further had has have having he her here
hers him his how i if in into is it its itself just me more most my no nor not now of off on once only or other our ours out
over own same she should so some such than that the their them then there these they this those through to too under until up
very was we were what when where which while who whom why will with would you your yours us
ability able accountable achieve achieving across additional advanced advantage advantageous aligned all-round ambitious
analytical apply applicants applicant appropriate area areas assigned assist assisting attention attitude available
background basic benefits best bonus bring broad build building  candidate candidates capability capable career
challenge challenging closely collaborate collaborative collaboration comfortable commitment committed communicate
communication communicator company competencies competency competent competitive comprehensive concerned considered
continuous contribute contribution coordinate core create creative critical culture current   daily day
days deadline deadlines dedicated deep deliver delivering  demonstrable demonstrated department description
desirable desired detail detailed details develop developing dynamic eager effective effectively efficient efficiently
employee employees encouraged ensure ensuring environment equivalent essential etc etc. excellent exceptional excellence
expected experience experienced experiences expert expertise exposure extensive familiar familiarity fast  flexible
fluent focus focused following full function functional functions further gain general genuine given good great grow growing
growth hands hands-on hardworking help helping high highly hire hiring ideal ideally identify impact implement implementing
important improve improvement include includes including independent independently individual  initiative initiatives
innovative interest interested internal interpersonal involve involved issues job join key knowledge large lead leading
learn learning least level levels like looking maintain maintaining make making manage managing matters meet meeting minimum
mindset motivated multiple must need needed needs new no. number objectives offer offering office oriented other others
ownership paced package part passion passionate people perform  person personal plan  plus position
positive possess practical practice practices prefer preferable preferably preferred prepare preparing present 
proactive problem problems   professional proficiency proficient progress   promote proven
provide providing purpose  quick qualification qualifications range ready record regarding related relationship
relationships relevant reliable    require required requirement requirements resolve responsibilities
responsibility responsible result results review reviewing right role roles salary scope self self-motivated seniors set
similar skill skilled skills solid   solve solving sound specific stakeholder stakeholders  
start strong success successful successfully support supporting take task tasks team teams thinking time timely together
 track train  two understand understanding upon use used using various verbal way ways well whole willing
willingness within without work worked working works world written year years`)

// short words that are terms, not noise.
var shortTerms = setOf(`ai ar ap bi hr it qa qc ui ux pr go c# c++ r 3d gl kl pj`)

func setOf(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

type token struct {
	raw, lower string
	breakAfter bool // a sentence or line ends after it: no phrase across it
}

func tokens(text string) []token {
	var out []token
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '.' || r == ';' || r == '•' || r == ':' || r == ',' }) {
		ws := wordRe.FindAllString(line, -1)
		for i, w := range ws {
			w = strings.Trim(w, "./-&")
			if w == "" {
				continue
			}
			out = append(out, token{raw: w, lower: strings.ToLower(w), breakAfter: i == len(ws)-1})
		}
	}
	return out
}

// useful is a word worth searching for: not generic, not a bare number, long enough or a known short term.
func useful(t token) bool {
	if generic[t.lower] {
		return false
	}
	if shortTerms[t.lower] {
		return true
	}
	letters := 0
	for _, r := range t.lower {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 3 || (letters >= 2 && isAcronym(t.raw))
}

// isAcronym is a word written in capitals (ACCA, SAP, CPA): a qualification, a system.
func isAcronym(raw string) bool {
	upper := 0
	for _, r := range raw {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			upper++
		}
	}
	return upper >= 2 && len([]rune(raw)) <= 8
}

// SearchTerms is what a job's possible candidates are looked for with: at
// most 30 terms, the heaviest first (title words, phrases, acronyms, then the
// description's words by how often it uses them).
func SearchTerms(j Job) []SearchTerm {
	weights := map[string]float32{}
	add := func(term string, w float32) {
		if w > weights[term] {
			weights[term] = w
		}
	}
	title := func(s string, w float32) {
		var words []string
		for _, t := range tokens(s) {
			if useful(t) {
				words = append(words, t.lower)
				add(t.lower, w)
			}
		}
		if len(words) >= 2 && len(words) <= 4 {
			add(strings.Join(words, " "), titlePhrase)
		}
	}
	title(j.Title, titleWeight)
	if t := trimmed(j.AdTitle); t != "" {
		title(t, titleWeight-1)
	}

	counts := map[string]int{}
	phrases := map[string]int{}
	for _, s := range []*string{j.Description, j.AdText} {
		ts := tokens(redact.WithoutSalary(trimmed(s)))
		for i, t := range ts {
			if !useful(t) {
				continue
			}
			counts[t.lower]++
			if isAcronym(t.raw) {
				add(t.lower, acronymWeight)
			}
			if i+1 < len(ts) && !t.breakAfter && useful(ts[i+1]) {
				phrases[t.lower+" "+ts[i+1].lower]++
			}
		}
	}
	for w, n := range counts {
		add(w, min(maxWordWeight, wordWeight+0.5*float32(n-1)))
	}
	// Two-word terms the description repeats, or that hold a title word, are the job's own.
	for p, n := range phrases {
		first, second, _ := strings.Cut(p, " ")
		if n >= 2 || weights[first] >= titleWeight-1 || weights[second] >= titleWeight-1 {
			add(p, phraseWeight)
		}
	}

	out := make([]SearchTerm, 0, len(weights))
	for t, w := range weights {
		out = append(out, SearchTerm{Text: t, Weight: w})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Weight != out[b].Weight {
			return out[a].Weight > out[b].Weight
		}
		return out[a].Text < out[b].Text
	})
	if len(out) > maxSearchTerms {
		out = out[:maxSearchTerms]
	}
	return out
}
