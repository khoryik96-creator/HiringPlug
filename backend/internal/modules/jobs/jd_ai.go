package jobs

// The AI's reading of a job description, merged with the rules': the AI
// tells the title from a letterhead or a slogan, finds the hiring company
// and the place in running text, and knows the kind of job. Pay never goes
// to the AI (the worker takes it out first), so the salary and the closing
// date stay the rules'. A text the AI gives that is not in the description
// is not used.

import (
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/quote"
)

// WithAI is the rules' details with the AI's where it read them from the text.
func WithAI(text string, d JDDetails, ai api.JdAiDetails) JDDetails {
	src := quote.Prepare(text)
	quoted := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" || !src.Has(v) {
			return ""
		}
		return clipRunes(v, 200)
	}
	if v, err := ai.Title.Get(); err == nil && quoted(v) != "" {
		d.Title = quoted(v)
	}
	if v, err := ai.CompanyName.Get(); err == nil && quoted(v) != "" {
		d.CompanyName = quoted(v)
	}
	if v, err := ai.Location.Get(); err == nil && quoted(v) != "" {
		d.Location = quoted(v)
		if c := countryOf(d.Location); c != "" {
			d.Country = c
		}
	}
	if v, err := ai.Country.Get(); err == nil && d.Country == "" && len(v) == 2 {
		d.Country = strings.ToUpper(v)
	}
	if v, err := ai.WorkType.Get(); err == nil && v.Valid() {
		d.WorkType = string(v)
	}
	if v, err := ai.WorkArrangement.Get(); err == nil && v.Valid() {
		d.WorkArrangement = string(v)
	}
	if v, err := ai.Openings.Get(); err == nil && v >= 1 && v <= 999 {
		d.Openings = &v
	}
	return d
}
