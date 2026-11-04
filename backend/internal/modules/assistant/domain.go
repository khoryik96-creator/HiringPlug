package assistant

// What the AI may see, as pure functions: a candidate, a job or an
// application turned into text sections, without contact details, names or
// salary, and the rules for a question.

import (
	"fmt"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// Candidate is a candidate as the asking user may see them (candidates
// module). There is no salary here: it is never sent to the AI.
type Candidate struct {
	ID string
	// Name, Email and Phone are only used to take them out of text.
	Name, Email, Phone string

	CurrentTitle, CurrentCompany, Location *string
	Industry, Specialization               *string
	WorkAuthorization                      *string // in words, e.g. "local"
	NoticePeriod                           *string // in words, e.g. "1 month"
	Skills                                 []string
	Work                                   []Work
	Education                              []Qualification
}

// Work is one job in a candidate's history.
type Work struct {
	Title, Company     *string
	StartDate, EndDate *string // YYYY-MM or YYYY
	Current            bool
	Description        *string
}

// Qualification is one entry of a candidate's education.
type Qualification struct {
	Qualification, Institution *string
	EndYear                    *int
}

// Job is a job as the asking user may see it, without its salary.
type Job struct {
	ID, Title, CompanyName, Status string
	Location, WorkType             *string
	WorkArrangement, Description   *string
	Openings                       int
}

// Application is a candidate for a job.
type Application struct {
	ID, CandidateID, JobID, Stage string
}

// Section is one part of what the AI sees.
type Section struct{ Title, Text string }

// Question is what the user asks.
type Question struct {
	SubjectType, SubjectID, Kind string
	Question                     *string
}

// Limits on what is sent, in characters.
const (
	maxCVText      = 24000
	maxDescription = 8000
)

// CheckQuestion tidies a question and checks it fits its kind and subject.
func CheckQuestion(q Question) (Question, error) {
	var errs apperr.FieldErrors
	if q.Question != nil {
		t := strings.TrimSpace(*q.Question)
		q.Question = &t
		if t == "" {
			q.Question = nil
		}
	}
	switch {
	case q.Kind == "question" && q.Question == nil:
		errs.Add("question", "write your question")
	case q.Kind != "question" && q.Question != nil:
		errs.Add("question", "only for your own question (kind question)")
	}
	if q.Kind == "fit" && q.SubjectType != "application" {
		errs.Add("kind", "fit is for a candidate's job (an application)")
	}
	if strings.TrimSpace(q.SubjectID) == "" {
		errs.Add("subjectId", "is required")
	}
	return q, errs.Err()
}

// MonthStart is when the month that holds now began, in loc.
func MonthStart(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}

// CandidateSections is what the AI sees of a candidate: the profile, work
// history, education and the CV's text (empty when none was read or the
// user may not see documents), each without contact details or names.
func CandidateSections(c Candidate, cvText string) []Section {
	id := redact.IdentityOf(c.Name, c.Email, c.Phone)
	clean := func(s string) string { return redact.WithoutSalary(redact.Scrub(s, id)) }

	var profile []string
	line := func(label string, v *string) {
		if v != nil && strings.TrimSpace(*v) != "" {
			profile = append(profile, label+": "+clean(*v))
		}
	}
	line("Current title", c.CurrentTitle)
	line("Current employer", c.CurrentCompany)
	line("Location", c.Location)
	line("Industry", c.Industry)
	line("Specialization", c.Specialization)
	line("Right to work", c.WorkAuthorization)
	line("Notice period", c.NoticePeriod)
	if len(c.Skills) > 0 {
		profile = append(profile, "Skills: "+clean(strings.Join(c.Skills, ", ")))
	}
	out := []Section{{Title: "Candidate profile", Text: orNone(strings.Join(profile, "\n"))}}

	if len(c.Work) > 0 {
		var b strings.Builder
		for _, w := range c.Work {
			fmt.Fprintf(&b, "- %s at %s (%s)\n", or(w.Title, "role not given"), or(w.Company, "employer not given"), dates(w))
			if w.Description != nil && strings.TrimSpace(*w.Description) != "" {
				b.WriteString("  " + strings.ReplaceAll(clip(clean(*w.Description), 2000), "\n", "\n  ") + "\n")
			}
		}
		out = append(out, Section{Title: "Work history (newest first)", Text: strings.TrimSpace(clean(b.String()))})
	}
	if len(c.Education) > 0 {
		var b strings.Builder
		for _, e := range c.Education {
			year := ""
			if e.EndYear != nil {
				year = fmt.Sprintf(" (%d)", *e.EndYear)
			}
			fmt.Fprintf(&b, "- %s, %s%s\n", or(e.Qualification, "qualification not given"), or(e.Institution, "institution not given"), year)
		}
		out = append(out, Section{Title: "Education", Text: strings.TrimSpace(clean(b.String()))})
	}
	if t := strings.TrimSpace(cvText); t != "" {
		out = append(out, Section{Title: "CV text (contact details and name removed)", Text: clip(clean(t), maxCVText)})
	}
	return out
}

// JobSections is what the AI sees of a job: never its salary.
func JobSections(j Job) []Section {
	lines := []string{"Title: " + j.Title, "Employer: " + j.CompanyName, "Status: " + j.Status, fmt.Sprintf("Openings: %d", j.Openings)}
	for _, f := range []struct {
		label string
		v     *string
	}{{"Location", j.Location}, {"Work type", j.WorkType}, {"Arrangement", j.WorkArrangement}} {
		if f.v != nil && *f.v != "" {
			lines = append(lines, f.label+": "+*f.v)
		}
	}
	out := []Section{{Title: "Job", Text: strings.Join(lines, "\n")}}
	if j.Description != nil && strings.TrimSpace(*j.Description) != "" {
		out = append(out, Section{Title: "Job description", Text: clip(redact.WithoutSalary(strings.TrimSpace(*j.Description)), maxDescription)})
	}
	return out
}

// ApplicationSections is a candidate for a job: the job, the candidate, and
// where they are in the process.
func ApplicationSections(a Application, c Candidate, cvText string, j Job) []Section {
	out := JobSections(j)
	out = append(out, Section{Title: "Where the candidate is in the hiring process", Text: "Stage: " + a.Stage})
	return append(out, CandidateSections(c, cvText)...)
}

func dates(w Work) string {
	from := or(w.StartDate, "?")
	switch {
	case w.Current:
		return from + " to now"
	case w.EndDate != nil:
		return from + " to " + *w.EndDate
	}
	return from
}

func or(s *string, fallback string) string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return fallback
	}
	return strings.TrimSpace(*s)
}

func orNone(s string) string {
	if s == "" {
		return "(nothing recorded)"
	}
	return s
}

// clip keeps the first n characters, saying so when it cuts.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "\n[... cut here]"
	}
	return s
}
