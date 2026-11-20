package interviewprep

// The rules of interview insights, as pure functions on plain values.

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// Question is a question asked at a job's interviews.
type Question struct {
	ID, JobID, Question, Kind string
	Round, Source, Notes      *string
	AskedOn                   *time.Time // a date
	Archived                  bool
	Version                   int
	CreatedAt, UpdatedAt      time.Time
}

// QuestionFields are what a person writes about a question.
type QuestionFields struct {
	Question, Kind       string
	Round, Source, Notes *string
	AskedOn              *time.Time
}

var spaces = regexp.MustCompile(`[ \t]+`)

// Clean trims what was written (blank optional fields become nil) and checks
// it: a question, and a date asked that is not ahead of today.
func Clean(f QuestionFields, today time.Time) (QuestionFields, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	f.Question = strings.TrimSpace(spaces.ReplaceAllString(f.Question, " "))
	if f.Question == "" {
		errs.Add("question", "write the question")
	} else if utf8.RuneCountInString(f.Question) > 2000 {
		errs.Add("question", "at most 2000 characters")
	}
	f.Source, f.Notes, f.Round = optional(f.Source), optional(f.Notes), optional(f.Round)
	if f.AskedOn != nil && f.AskedOn.After(today) {
		errs.Add("askedOn", "cannot be ahead of today")
	}
	return f, errs
}

func optional(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// MaxJobText is how much of the job's description and ad goes to the AI.
const MaxJobText = 12000

// MaxRecordedQuestions is how many recorded questions (the newest) one prep
// covers: each becomes a card, and the AI's answer has a set length.
const MaxRecordedQuestions = 12

var (
	// money is an amount with its currency, e.g. "RM 8,000" or "SGD7k".
	money = regexp.MustCompile(`(?i)(?:\brm|\bmyr|\bsgd|\busd|\bidr|\beur|\bgbp|s\$|\$)\s?\d`)
	// payCue is a word saying a line is about pay, even without "salary":
	// "Package: 12,000 - 15,000 monthly", "Budget up to 15k".
	payCue = regexp.MustCompile(`(?i)\b(?:package|budget|monthly|per\s+month|p\.\s?m\.?|/\s?month|per\s+annum|p\.\s?a\.?|annually|per\s+year|up\s+to|wages?|allowance|bonus|ctc|pay)\b`)
	// amount is a figure written as pay is: 12,000 / 8.5k / 9000 (not a year).
	amount = regexp.MustCompile(`(?i)\b\d{1,3}(?:,\d{3})+(?:\.\d+)?\b|\b\d+(?:\.\d+)?\s?k\b|\b\d{4,7}\b`)
	year   = regexp.MustCompile(`^(?:19|20)\d\d$`)
)

// aboutPay reports a line naming pay: an amount with a currency, or an
// amount on a line with a pay word.
func aboutPay(line string) bool {
	if money.MatchString(line) {
		return true
	}
	if !payCue.MatchString(line) {
		return false
	}
	for _, a := range amount.FindAllString(line, -1) {
		if !year.MatchString(a) {
			return true
		}
	}
	return false
}

// JobText is what the AI is told of the job: its description and its ad's
// text (a line the ad repeats, once), without salary: lines naming pay are
// cut at the salary word, and lines with a currency amount, or an amount
// beside a pay word (package, budget, monthly, up to...), left out.
func JobText(description, ad string) string {
	description, ad = strings.TrimSpace(description), strings.TrimSpace(ad)
	known := map[string]bool{}
	for _, line := range strings.Split(description, "\n") {
		known[strings.TrimSpace(line)] = true
	}
	var extra []string // the ad's lines the description does not already have
	for _, line := range strings.Split(ad, "\n") {
		if t := strings.TrimSpace(line); t == "" || !known[t] {
			extra = append(extra, line)
		}
	}
	text := description
	if more := strings.TrimSpace(strings.Join(extra, "\n")); more != "" {
		if text != "" {
			text += "\n\n"
		}
		text += more
	}
	var kept []string
	for _, line := range strings.Split(redact.WithoutSalary(text), "\n") {
		if !aboutPay(line) {
			kept = append(kept, line)
		}
	}
	text = strings.TrimSpace(strings.Join(kept, "\n"))
	if utf8.RuneCountInString(text) > MaxJobText {
		text = string([]rune(text)[:MaxJobText])
	}
	return text
}

// EnoughToPrepare reports whether there is anything to prepare from: some of
// the job's text, or a recorded question.
func EnoughToPrepare(jobText string, questions int) bool {
	return utf8.RuneCountInString(strings.TrimSpace(jobText)) >= 40 || questions > 0
}

// GivesUpAfter is how long prep material may stay queued before it shows as
// failed: its AI job died and no answer will come (a late one is still used).
const GivesUpAfter = 30 * time.Minute

// Shown is prep material as it stands at now.
func Shown(p Prep, now time.Time) Prep {
	if p.Status == "queued" && now.Sub(p.CreatedAt) > GivesUpAfter {
		reason := "It took too long: the AI did not answer. Try again."
		p.Status, p.Error = "failed", &reason
	}
	return p
}

func lines(in []string, most, size int) []string {
	out := []string{}
	for _, s := range in {
		if s = clip(s, size); s != "" {
			out = append(out, s)
		}
		if len(out) == most {
			break
		}
	}
	return out
}

func clip(s string, size int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if utf8.RuneCountInString(s) > size {
		s = strings.TrimSpace(string([]rune(s)[:size-1])) + "…"
	}
	return s
}
