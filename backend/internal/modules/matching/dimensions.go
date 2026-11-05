package matching

// CV Studio's CV scoring, as AI notes beside the score: six ratings with a
// note each, strengths and gaps. They are the AI's opinion; the score itself
// is still worked out from the quoted requirements, and the recommendation
// from the score, with CV Studio's thresholds.

import (
	"slices"
	"strings"
)

// DimensionNames are CV Studio's six ratings, in the order they are shown.
var DimensionNames = []string{"skills", "experience", "industry", "education", "leadership", "keywords"}

// Dimension is one rating, 0 to 100, with the AI's reason.
type Dimension struct {
	Name  string  `json:"name"`
	Score int     `json:"score"`
	Note  *string `json:"note"`
}

const maxNotes = 5 // strengths, and gaps

// CheckDimensions keeps each known rating once (the first), in CV Studio's
// order, with its score held to 0..100 and its note trimmed.
func CheckDimensions(in []Dimension) []Dimension {
	out := []Dimension{}
	for _, name := range DimensionNames {
		i := slices.IndexFunc(in, func(d Dimension) bool { return d.Name == name })
		if i < 0 {
			continue
		}
		d := in[i]
		out = append(out, Dimension{Name: name, Score: min(max(d.Score, 0), 100), Note: clipped(d.Note, maxNote)})
	}
	return out
}

// CheckNotes keeps up to five distinct, non-empty notes, each trimmed.
func CheckNotes(in []string) []string {
	out := []string{}
	for _, n := range in {
		n = clip(strings.Join(strings.Fields(n), " "), maxNote)
		if n == "" || slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, n) }) {
			continue
		}
		if out = append(out, n); len(out) == maxNotes {
			break
		}
	}
	return out
}

// Recommendation is what CV Studio advises for a score: shortlist at 70 or
// more, consider from 45, pass below; nil without a score.
func Recommendation(score *int) *string {
	switch {
	case score == nil:
		return nil
	case *score >= 70:
		return ptr("shortlist")
	case *score >= 45:
		return ptr("consider")
	default:
		return ptr("pass")
	}
}
