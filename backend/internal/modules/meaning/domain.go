package meaning

// The ranking, as a recruiter weighs a candidate: first what they do (their
// titles, read as job families: the newest counts most, the level asked for
// first), then what their CV says, then their profile as a whole, and the
// search's own words in their CV. Pure: unit tested.
//
// The weights are measured on recruiter benches (made-up candidates and
// searches with the traps a recruiter would notice) by
// ai/tests/test_meaning_bench.py, which ranks with these same numbers:
// change both together.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	titleOnlyRole, titleOnlyProfile                    = 0.7, 0.3
	contextRole, contextPassage, contextProfile        = 0.45, 0.35, 0.2
	describesPassage, describesRole                    = 0.5, 0.5
	describesNoFamilyPassage, describesNoFamilyProfile = 0.75, 0.25
	jobRole, jobNeeds, jobProfile                      = 0.55, 0.25, 0.2
	wordsWeight, exactWordsWeight                      = 0.06, 0.25 // the search's own words in the CV

	levelStep, levelMaxSteps = 0.03, 3    // a level away from the one asked for
	oldRoleStep, oldRoleLow  = 0.05, 0.55 // a year since a title ended, and the least it keeps

	// What a score says (measured on the benches: right answers mostly
	// above Strong, wrong ones almost all below Shown).
	Strong, Good, Shown = 0.65, 0.5, 0.38
)

// Query is how a search or a job was read.
type Query struct {
	HasRole    bool // the role asked for was read (a family, for a search describing work)
	HasContext bool // the search says more than the role ("for finance roles")
	Describes  bool // it describes work rather than naming a title
	Exact      bool // it names a tool, qualification or code (S/4HANA, ACCA)
	Level      *int // the level asked for
	Needs      int  // a job's needs (its description's lines), each compared with CVs
}

// RoleSim is one title a candidate held, compared with the role asked for.
type RoleSim struct {
	Title   string
	Family  *string
	Level   *int
	EndYear *int // nil: their job now
	Field   float64
	Work    float64
}

// Evidence is what was compared for one candidate: each number is a
// similarity from the database (about 0 to 1).
type Evidence struct {
	Roles          []RoleSim
	PassageWhole   float64 // their CV's passage nearest the whole search
	PassageContext float64 // ... nearest what else it says
	Passage        string  // the passage nearest the search (or its context)
	Profile        float64
	Needs          []float64 // for a job: each need, against their CV's nearest passage
	Words          float64   // the search's own words in their CV, 0 to 1
	WordsFound     []string
}

// Scored is a candidate's score and what made it.
type Scored struct {
	Score       float64
	Role        *RoleSim // their title that matched best (nil: none)
	RoleScore   float64
	ShowPassage bool // their CV mattered: show the passage
}

// Recency is what a title counts for: 1 for their job now, less each year
// since it ended, never below oldRoleLow (it is still their experience).
func Recency(endYear *int, thisYear int) float64 {
	if endYear == nil {
		return 1
	}
	return math.Max(oldRoleLow, 1-oldRoleStep*float64(thisYear-*endYear))
}

// LevelFit is 1 at the level asked for, a little less each level away.
func LevelFit(want, have *int) float64 {
	if want == nil || have == nil {
		return 1
	}
	steps := math.Min(levelMaxSteps, math.Abs(float64(*want-*have)))
	return 1 - levelStep*steps
}

// Score ranks one candidate for a query, in the year given.
func Score(q Query, e Evidence, thisYear int) Scored {
	var out Scored
	for i := range e.Roles {
		r := &e.Roles[i]
		s := math.Max(r.Field, r.Work) * Recency(r.EndYear, thisYear) * LevelFit(q.Level, r.Level)
		if out.Role == nil || s > out.RoleScore {
			out.Role, out.RoleScore = r, s
		}
	}
	role := 0.0
	if q.HasRole {
		role = out.RoleScore
	}
	switch {
	case q.Needs > 0:
		met := 0.0
		for _, n := range e.Needs {
			met += n
		}
		met /= float64(q.Needs) // a need their CV says nothing about counts as unmet
		out.Score = jobRole*role + jobNeeds*met + jobProfile*e.Profile
		out.ShowPassage = met > 0
	case q.Describes && q.HasRole:
		out.Score = describesPassage*e.PassageWhole + describesRole*role
		out.ShowPassage = true
	case q.Describes:
		out.Score = describesNoFamilyPassage*e.PassageWhole + describesNoFamilyProfile*e.Profile
		out.ShowPassage = true
	case q.HasContext:
		out.Score = contextRole*role + contextPassage*e.PassageContext + contextProfile*e.Profile
		out.ShowPassage = e.PassageContext > 0
	default:
		out.Score = titleOnlyRole*role + titleOnlyProfile*e.Profile
	}
	if q.Exact {
		out.Score += exactWordsWeight * e.Words
	} else {
		out.Score += wordsWeight * e.Words
	}
	if !q.HasRole {
		out.Role = nil
	}
	return out
}

// Strength says a score in words.
func Strength(score float64) string {
	switch {
	case score >= Strong:
		return "strong"
	case score >= Good:
		return "good"
	default:
		return "possible"
	}
}

// When says when a title was theirs: "now", or "until 2019".
func When(endYear *int) string {
	if endYear == nil {
		return "now"
	}
	return "until " + strconv.Itoa(*endYear)
}

// LevelNames are the levels' names (the AI worker reads titles with the same).
var LevelNames = map[int]string{
	0: "Intern or trainee", 1: "Junior or assistant", 2: "Executive", 3: "Senior",
	4: "Assistant manager", 5: "Manager", 6: "Senior manager", 7: "Director and above",
}

var spaces = regexp.MustCompile(`\s+`)

// QueryHash identifies a search's words (or a job's title and text), the
// same however they are spaced or capitalised, so they are read once.
func QueryHash(title, body string) string {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(spaces.ReplaceAllString(s, " "))) }
	sum := sha256.Sum256([]byte(norm(title) + "\n" + norm(body)))
	return hex.EncodeToString(sum[:])
}

// Fingerprint identifies what is read of a candidate: their titles and work,
// and their newest CV (which one, and its text). A change in any of them
// means reading them again.
func Fingerprint(s Source) string {
	type cv struct{ ID, Text string }
	var c *cv
	if s.CV != nil {
		sum := sha256.Sum256([]byte(s.CV.Text))
		c = &cv{s.CV.DocumentID, hex.EncodeToString(sum[:8])}
	}
	data, _ := json.Marshal(struct { //nolint:errchkjson // plain strings and ints
		Title string
		Work  []Work
		CV    *cv
	}{s.CurrentTitle, s.Work, c})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// markedWords are the words a CV snippet marks [[like this]].
var markedWords = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// WordsIn returns the distinct words a snippet marks, as they appear.
func WordsIn(snippet string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range markedWords.FindAllStringSubmatch(snippet, -1) {
		w := strings.TrimSpace(m[1])
		if k := strings.ToLower(w); w != "" && !seen[k] {
			seen[k] = true
			out = append(out, w)
		}
	}
	return out
}

// stopWords are left out of a search's own words: they say nothing about who fits.
var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the and or of for in with to at who that on from by as is are was be someone somebody
		person people candidate candidates anyone hired prepared managed led worked has have had experience years year role roles
		job jobs work senior junior manager managers executive officer head lead assistant specialist associate staff team good
		strong`) {
		stopWords[w] = true
	}
}

var searchWord = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}+#./&-]*[\p{L}\p{N}+#]|[\p{L}\p{N}]{2,}`)

// SearchWords are a search's own distinctive words (and its codes, such as
// S/4HANA or C++), to find in CVs: at most eight.
func SearchWords(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range searchWord.FindAllString(text, -1) {
		k := strings.ToLower(w)
		if stopWords[k] || seen[k] || len(out) == 8 {
			continue
		}
		seen[k] = true
		out = append(out, w)
	}
	return out
}
