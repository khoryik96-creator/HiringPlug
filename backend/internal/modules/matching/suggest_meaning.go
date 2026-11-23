package matching

// Possible candidates by meaning: the job's title and the needs its
// description lists, read as a recruiter reads them (the meaning module),
// with the job's distinctive words in CVs counted too. Those found by
// meaning come first, each with why; then those found only by the words.

import (
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// MeaningMatch is a candidate found for a job by meaning.
type MeaningMatch struct {
	CandidateID string
	Score       float64
	Why         api.MeaningWhy
}

// JobMeaningText is what a job is read by meaning from, besides its title:
// its description and its ad (not its employer or place: those say nothing
// about who fits).
func JobMeaningText(j Job) string {
	var parts []string
	for _, s := range []*string{j.Description, j.AdText} {
		if t := trimmed(s); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// suggestionOrder is who to suggest, in order: by meaning first, then
// those found only by the job's words.
func suggestionOrder(byMeaning []MeaningMatch, byWords []CVHit) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range byMeaning {
		if !seen[m.CandidateID] {
			seen[m.CandidateID] = true
			out = append(out, m.CandidateID)
		}
	}
	for _, h := range byWords {
		if !seen[h.CandidateID] {
			seen[h.CandidateID] = true
			out = append(out, h.CandidateID)
		}
	}
	return out
}
