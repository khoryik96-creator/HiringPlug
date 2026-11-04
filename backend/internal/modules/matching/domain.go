package matching

// The rules, as pure functions: what the AI is sent (the job's text and the
// CV's text, without names, contact details or salary), how its answer is
// checked against what it was sent, and the score worked out from what is
// left. The AI never sets the score itself.

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/quote"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// Candidate is what identifies a candidate, only to take it out of their
// CV's text. Salary is never here.
type Candidate struct{ Name, Email, Phone string }

// Job is a job as the AI sees it: never its salary budget.
type Job struct {
	ID, Title, CompanyName              string
	Location, WorkType, WorkArrangement *string
	Description                         *string
	AdTitle, AdText                     *string // the job's ad, if it has one
}

// Application is a candidate on a job.
type Application struct{ ID, CandidateID, JobID, Stage string }

// Verdicts: how much of a requirement the CV shows.
const (
	Met     = "met"
	Partly  = "partly"
	Missing = "missing"
)

// Claim is one requirement as the AI read it, not yet checked.
type Claim struct {
	Requirement, Verdict string
	Evidence, Note       *string
}

// Reason is a checked requirement: its quote is in the job's text, and it
// counts as met (or partly) only with evidence quoted from the CV.
type Reason struct {
	Requirement string  `json:"requirement"`
	Verdict     string  `json:"verdict"`
	Evidence    *string `json:"evidence"`
	Note        *string `json:"note"`
}

// Limits, in characters.
const (
	maxCVText       = 24000
	maxJobText      = 12000
	maxQuote        = 500
	maxNote         = 300
	maxSummary      = 600
	maxRequirements = 15
)

// Notes on a requirement whose verdict was changed by the check.
const (
	noteNoEvidence  = "Counted as not shown: the AI gave no quote from the CV for it."
	noteBadEvidence = "Counted as not shown: the AI's quote from the CV is not in the CV."
)

// JobText is what the AI reads of a job: its title and details, its
// description and its ad, without salary. ok is false when there is neither
// a description nor an ad text to compare with.
func JobText(j Job) (text string, ok bool) {
	var b strings.Builder
	b.WriteString("Job title: " + j.Title)
	if j.CompanyName != "" {
		b.WriteString("\nEmployer: " + j.CompanyName)
	}
	for _, f := range []struct {
		label string
		v     *string
	}{{"Location", j.Location}, {"Work type", j.WorkType}, {"Arrangement", j.WorkArrangement}} {
		if s := trimmed(f.v); s != "" {
			b.WriteString("\n" + f.label + ": " + s)
		}
	}
	if s := trimmed(j.Description); s != "" {
		b.WriteString("\n\nJob description:\n" + redact.WithoutSalary(s))
		ok = true
	}
	if s := trimmed(j.AdText); s != "" {
		b.WriteString("\n\nJob ad")
		if t := trimmed(j.AdTitle); t != "" {
			b.WriteString(" (" + t + ")")
		}
		b.WriteString(":\n" + redact.WithoutSalary(s))
		ok = true
	}
	return clip(b.String(), maxJobText), ok
}

// CVText is what the AI reads of a CV: its text without the candidate's
// name, contact details, profile links or salary ("" for no text).
func CVText(c Candidate, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return clip(redact.WithoutSalary(redact.Scrub(text, redact.IdentityOf(c.Name, c.Email, c.Phone))), maxCVText)
}

// personal finds requirements about who a person is rather than what they
// can do. They are never scored, whatever the job ad says.
// ("Health and safety" or "race conditions" are skills, so those words alone
// do not count.)
var personal = regexp.MustCompile(`(?i)\b(?:age[ds]?\s*:?\s*(?:\d+|between|below|under|above|not\b)|years?\s+old|male|female|gender|sex|racial|ethnic\w*|bumiputera|religio\w*|muslim|christian|hindu|buddhist|married|marital|single\s+status|nationality|citizen\w*|malaysians?\s+only|permanent\s+residen\w*|pregnan\w*)\b`)

// Check keeps the AI's requirements whose quote is in the job's text (the
// others are counted as dropped), never more than 15, the same one only
// once, and none about who a person is (age, gender, race, religion,
// marital status, nationality). A requirement counts as met or
// partly met only with evidence that is in the CV's text; without it, it
// counts as missing, with a note saying why.
func Check(jobText, cvText string, claims []Claim) (reasons []Reason, dropped int) {
	job, cv := quote.Prepare(jobText), quote.Prepare(cvText)
	seen := map[quote.Source]bool{}
	for _, c := range claims {
		req := strings.TrimSpace(c.Requirement)
		verdict := c.Verdict
		if utf8.RuneCountInString(req) > maxQuote || !job.Has(req) || (verdict != Met && verdict != Partly && verdict != Missing) {
			dropped++
			continue
		}
		key := quote.Prepare(req)
		if seen[key] || personal.MatchString(req) {
			continue
		}
		seen[key] = true
		if len(reasons) == maxRequirements {
			break
		}
		r := Reason{Requirement: req, Verdict: verdict, Note: clipped(c.Note, maxNote)}
		evidence := trimmed(c.Evidence)
		switch {
		case evidence != "" && utf8.RuneCountInString(evidence) <= maxQuote && cv.Has(evidence):
			r.Evidence = &evidence
		case verdict == Missing:
			// nothing claimed, nothing to check
		case evidence == "":
			r.Verdict, r.Note = Missing, ptr(noteNoEvidence)
		default:
			r.Verdict, r.Note = Missing, ptr(noteBadEvidence)
		}
		reasons = append(reasons, r)
	}
	return reasons, dropped
}

// Score is how much of the job's requirements the CV shows, from 0 to 100:
// met counts in full, partly as half. ok is false with no requirements.
func Score(reasons []Reason) (score int, ok bool) {
	if len(reasons) == 0 {
		return 0, false
	}
	halves := 0
	for _, r := range reasons {
		switch r.Verdict {
		case Met:
			halves += 2
		case Partly:
			halves++
		}
	}
	return int(math.Round(100 * float64(halves) / float64(2*len(reasons)))), true
}

// Visible is what a user sees of the reasons: the evidence quotes the CV, so
// it is left out for those who may not see CVs.
func Visible(reasons []Reason, canSeeCVs bool) []Reason {
	if canSeeCVs {
		return reasons
	}
	out := make([]Reason, len(reasons))
	for i, r := range reasons {
		r.Evidence = nil
		out[i] = r
	}
	return out
}

// Summary tidies the AI's overall view (nil for none).
func Summary(s *string) *string { return clipped(s, maxSummary) }

func trimmed(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func clipped(s *string, n int) *string {
	t := trimmed(s)
	if t == "" {
		return nil
	}
	if r := []rune(t); len(r) > n {
		t = strings.TrimSpace(string(r[:n-1])) + "…"
	}
	return &t
}

// clip keeps the first n characters, saying so when it cuts.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "\n[... cut here]"
	}
	return s
}

func ptr(s string) *string { return &s }
