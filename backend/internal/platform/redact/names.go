package redact

// A person's name in text, by CV Studio's rules (its engine is the Python AI
// worker's ai/cvstudio): the whole name is found in any case, but a single
// name word only where it reads as the person. May, Grace, Jaya, Abdul and
// Do are also months, nouns, places, lineage and verbs, so a single word
// counts only when it is written with a capital, is not one of CV Studio's
// everyday, place, lineage or technology words, has three letters or more,
// and does not follow a lineage word ("binti Rahman" is the father). A
// one-word name is taken even when it is an everyday word, except before a
// number ("May 2023"). Scripts without capitals (Chinese, Tamil, Arabic...)
// count from two letters, and Chinese, Japanese and Thai, written without
// spaces, also inside a longer run of text.

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed name_words.json
var nameWordsJSON []byte

type nameWords struct {
	MinLetters   int      `json:"minLetters"`
	Common       []string `json:"common"`
	Places       []string `json:"places"`
	Placeholders []string `json:"placeholders"`
	Particles    []string `json:"particles"`
	Technology   []string `json:"technology"`
	OrgTech      []string `json:"orgTech"`
	Honorifics   []string `json:"honorifics"`
}

var words = func() nameWords {
	var w nameWords
	if err := json.Unmarshal(nameWordsJSON, &w); err != nil {
		panic("redact: name_words.json: " + err.Error())
	}
	return w
}()

// Lineage words HiringPlug always treated as not identifying alone, besides
// CV Studio's (bt, s/o, d/o, al, el, de).
var moreParticles = []string{"bt", "s/o", "d/o", "al", "el", "de"}

func isParticle(w string) bool {
	w = strings.ToLower(w)
	return slices.Contains(words.Particles, w) || slices.Contains(moreParticles, w)
}

func isHonorific(w string) bool {
	return slices.Contains(words.Honorifics, strings.ToLower(strings.TrimSuffix(w, ".")))
}

// cased reports a letter from a script with capitals (Latin, Greek...).
func cased(r rune) bool { return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) }

// unspaced reports a word in a script written without spaces between words.
func unspaced(w string) bool {
	for _, r := range w {
		if !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai) {
			return false
		}
	}
	return w != ""
}

// everyday reports a word that is also ordinary text, a place, a lineage
// word, a technology or too short to be sure of: never matched alone.
func everyday(w string) bool {
	low := strings.ToLower(w)
	least := words.MinLetters
	if first, _ := utf8.DecodeRuneInString(w); !cased(first) {
		least = 2 // "伟明": a two-letter given name
	}
	if utf8.RuneCountInString(w) < least {
		return true
	}
	for _, list := range [][]string{words.Common, words.Places, words.Placeholders, words.Particles, words.Technology, words.OrgTech} {
		if slices.Contains(list, low) {
			return true
		}
	}
	return slices.Contains(moreParticles, low)
}

// splitName is a full name's words in order, without honorifics or the
// punctuation around them.
func splitName(name string) []string {
	var out []string
	for _, part := range strings.Fields(name) {
		part = strings.Trim(part, ".,;:|()[]'\"")
		if part != "" && !isHonorific(part) {
			out = append(out, part)
		}
	}
	return out
}

// isWordRune is a letter, digit or mark (Tamil and Hindi vowel signs are marks).
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) }

// compiled keeps the patterns of recent names, so checking a CV's many
// parts does not compile the same ones again (bounded: emptied when full).
var compiled = struct {
	sync.Mutex
	m map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

func pattern(expr string) *regexp.Regexp {
	compiled.Lock()
	defer compiled.Unlock()
	if re, ok := compiled.m[expr]; ok {
		return re
	}
	if len(compiled.m) >= 512 {
		compiled.m = map[string]*regexp.Regexp{}
	}
	re := regexp.MustCompile(expr)
	compiled.m[expr] = re
	return re
}

// whole reports a match that is a word on its own: not part of a longer one
// (in scripts without spaces any place counts).
func whole(text string, m []int, word string) bool {
	if unspaced(word) {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
	after, _ := utf8.DecodeRuneInString(text[m[1]:])
	return (m[0] == 0 || !isWordRune(before)) && (m[1] == len(text) || !isWordRune(after))
}

// wordMatches is every place a word stands whole in text, in any case.
func wordMatches(text, word string) [][2]int {
	var out [][2]int
	for _, m := range pattern(`(?i)`+regexp.QuoteMeta(word)).FindAllStringIndex(text, -1) {
		if whole(text, m, word) {
			out = append(out, [2]int{m[0], m[1]})
		}
	}
	return out
}

var (
	wordBefore     = regexp.MustCompile(`([\p{L}'’/-]+)[ \t]+$`)
	numberFollows  = regexp.MustCompile(`^[\s,\x{2013}\x{2014}-]*\d`)
	phraseBoundary = `[ \t\r\n]+`
)

// nameSpans is where text names the person: the whole name, and single
// words by the rules above, as sorted byte ranges that do not overlap.
func (id Identity) nameSpans(text string) [][2]int {
	var spans [][2]int
	if len(id.Full) >= 2 {
		parts := make([]string, len(id.Full))
		for i, p := range id.Full {
			parts[i] = regexp.QuoteMeta(p)
		}
		for _, m := range pattern(`(?i)`+strings.Join(parts, phraseBoundary)).FindAllStringIndex(text, -1) {
			if whole(text, m, strings.Join(id.Full, "")) {
				spans = append(spans, [2]int{m[0], m[1]})
			}
		}
	}
	mononym := len(id.Full) == 1
	for _, n := range id.Names {
		common := everyday(n)
		if common && !mononym {
			continue
		}
		for _, m := range wordMatches(text, n) {
			first, _ := utf8.DecodeRuneInString(text[m[0]:])
			if cased(first) && !unicode.IsUpper(first) { // "a grace period", "able to do"; 伟明 has no capitals
				continue
			}
			if b := wordBefore.FindStringSubmatch(text[:m[0]]); b != nil && isParticle(b[1]) {
				continue // "binti Rahman": the father's name, shared by the family
			}
			if mononym && common && numberFollows.MatchString(text[m[1]:]) {
				continue // "May 2023"
			}
			spans = append(spans, m)
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		return spans[i][0] < spans[j][0] || (spans[i][0] == spans[j][0] && spans[i][1] > spans[j][1])
	})
	var out [][2]int
	for _, s := range spans {
		if len(out) > 0 && s[0] < out[len(out)-1][1] {
			if s[1] > out[len(out)-1][1] {
				out[len(out)-1][1] = s[1]
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// replaceNames puts label wherever text names the person.
func (id Identity) replaceNames(text, label string) string {
	spans := id.nameSpans(text)
	for i := len(spans) - 1; i >= 0; i-- {
		text = text[:spans[i][0]] + label + text[spans[i][1]:]
	}
	return text
}
