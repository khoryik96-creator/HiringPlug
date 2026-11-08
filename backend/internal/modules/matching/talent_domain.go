package matching

// AI candidate search (CV Studio's AI Crawler), as pure functions: what the
// AI is told the job needs when there is no job description, and its
// ranking checked against who it was sent.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Limits of an AI candidate search.
const (
	TalentPool        = 300  // the most candidates the boolean and filters bring in
	TalentRankDefault = 15   // how many the AI ranks unless asked otherwise
	talentCVText      = 3500 // each CV is cut to this many characters for the AI
	// TalentGivesUpAfter is how long a search may wait for the AI before it is
	// shown as failed (its AI job died); an answer that still comes is used.
	TalentGivesUpAfter = 30 * time.Minute
)

// PoolCandidate is a candidate the boolean and filters found: their ID and
// what the AI may read of their profile (no name, contact or salary).
type PoolCandidate struct {
	ID      string
	Profile string
}

// Ref is how the AI names the i-th candidate sent (C1, C2...): never by name.
func Ref(i int) string { return fmt.Sprintf("C%d", i+1) }

// TalentRequirements is what the AI is told the job needs: the job
// description, or without one the boolean and the filters (never a salary).
func TalentRequirements(jd string, s api.CandidateSearch) string {
	if jd = strings.TrimSpace(jd); jd != "" {
		return jd
	}
	var b strings.Builder
	b.WriteString("No job description was given. The recruiter is looking for candidates matching:\n")
	if s.Boolean != nil && strings.TrimSpace(*s.Boolean) != "" {
		b.WriteString("- Boolean search: " + strings.TrimSpace(*s.Boolean) + "\n")
	}
	list := func(name string, v *[]string) {
		if v != nil && len(*v) > 0 {
			b.WriteString("- " + name + ": " + strings.Join(*v, ", ") + "\n")
		}
	}
	list("Skills", s.Skills)
	list("Industry", s.Industry)
	if s.Location != nil && strings.TrimSpace(*s.Location) != "" {
		b.WriteString("- Location: " + strings.TrimSpace(*s.Location) + "\n")
	}
	if s.NoticeMaxDays != nil {
		fmt.Fprintf(&b, "- Notice period: at most %d days\n", *s.NoticeMaxDays)
	}
	return strings.TrimRight(b.String(), "\n")
}

// TalentOrder is who to send first: those whose CVs share most words with
// the job description (ranked), then the rest of the pool in its order.
func TalentOrder(pool []PoolCandidate, ranked []string) []PoolCandidate {
	byID := make(map[string]PoolCandidate, len(pool))
	for _, c := range pool {
		byID[c.ID] = c
	}
	out := make([]PoolCandidate, 0, len(pool))
	seen := map[string]bool{}
	for _, id := range ranked {
		if c, ok := byID[id]; ok && !seen[id] {
			out = append(out, c)
			seen[id] = true
		}
	}
	for _, c := range pool {
		if !seen[c.ID] {
			out = append(out, c)
			seen[c.ID] = true
		}
	}
	return out
}

// TalentResult is one candidate's place in the AI's ranking, as saved.
type TalentResult struct {
	CandidateID string   `json:"candidateId"`
	Fit         int      `json:"fit"`
	Verdict     string   `json:"verdict"`
	Reasons     []string `json:"reasons"`
	Gaps        []string `json:"gaps"`
}

// TalentResults checks the AI's ranking against who it was sent (C1 is
// sent[0]): unknown or repeated references are dropped, fit is kept within
// 0-100 and the verdict follows it when the AI's is missing; best first.
func TalentResults(sent []string, ranked []api.TalentRanked) []TalentResult {
	out := []TalentResult{}
	seen := map[string]bool{}
	for _, r := range ranked {
		var n int
		if _, err := fmt.Sscanf(strings.ToUpper(strings.TrimSpace(r.Ref)), "C%d", &n); err != nil || n < 1 || n > len(sent) {
			continue
		}
		id := sent[n-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		fit := min(max(r.Fit, 0), 100)
		verdict := string(r.Verdict)
		if !r.Verdict.Valid() {
			verdict = verdictFor(fit)
		}
		out = append(out, TalentResult{CandidateID: id, Fit: fit, Verdict: verdict, Reasons: cleanList(r.Reasons), Gaps: cleanList(r.Gaps)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fit > out[j].Fit })
	return out
}

func verdictFor(fit int) string {
	switch {
	case fit >= 75:
		return string(api.TalentVerdictStrong)
	case fit >= 50:
		return string(api.TalentVerdictPossible)
	default:
		return string(api.TalentVerdictWeak)
	}
}

func cleanList(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			out = append(out, clip(s, 400))
		}
	}
	return out
}
