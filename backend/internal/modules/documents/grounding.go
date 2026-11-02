package documents

import (
	"unicode/utf8"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// groundProfile keeps only the values of p whose quote is in the CV text and
// whose value can be read from the quote (rules in domain.go), and
// counts what it dropped. Jobs and schools left with nothing are dropped.
func groundProfile(text string, p api.ParsedProfile) api.ParsedProfile {
	g := grounder{text: ForQuotes(text)}
	out := api.ParsedProfile{
		Parser:       p.Parser,
		FullName:     g.one(p.FullName, FieldText),
		Email:        g.one(p.Email, FieldText),
		Phone:        g.one(p.Phone, FieldText),
		LinkedinUrl:  g.one(p.LinkedinUrl, FieldURL),
		Location:     g.one(p.Location, FieldText),
		Nationality:  g.one(p.Nationality, FieldCountry),
		NoticePeriod: g.one(p.NoticePeriod, FieldNoticePeriod),
		Skills:       []api.SourcedText{},
		Experience:   []api.ParsedJob{},
		Education:    []api.ParsedEducation{},
	}
	for _, s := range p.Skills {
		if g.ok(s, FieldText) && len(out.Skills) < 100 {
			out.Skills = append(out.Skills, s)
		}
	}
	for _, j := range p.Experience {
		j.Title, j.Company, j.Dates = g.one(j.Title, FieldText), g.one(j.Company, FieldText), g.one(j.Dates, FieldText)
		if !j.Dates.IsSpecified() || j.Dates.IsNull() {
			j.StartDate, j.EndDate = nullable.NewNullNullable[string](), nullable.NewNullNullable[string]()
		}
		if (isSet(j.Title) || isSet(j.Company)) && len(out.Experience) < 30 {
			out.Experience = append(out.Experience, j)
		}
	}
	for _, e := range p.Education {
		e.Qualification, e.Institution, e.Dates = g.one(e.Qualification, FieldText), g.one(e.Institution, FieldText), g.one(e.Dates, FieldText)
		if !isSet(e.Dates) {
			e.EndYear = nullable.NewNullNullable[int]()
		}
		if (isSet(e.Qualification) || isSet(e.Institution)) && len(out.Education) < 15 {
			out.Education = append(out.Education, e)
		}
	}
	out.Dropped = g.dropped
	return out
}

type grounder struct {
	text    string
	dropped int
}

func (g *grounder) ok(s api.SourcedText, kind string) bool {
	// Limits in characters, as the contract counts them (not bytes: Chinese or accented text is longer in bytes).
	keep := utf8.RuneCountInString(s.Value) <= 300 && utf8.RuneCountInString(s.Quote) <= 1000 &&
		Quotes(g.text, s.Quote) && ValueFromQuote(kind, s.Value, s.Quote)
	if !keep {
		g.dropped++
	}
	return keep
}

func (g *grounder) one(v nullable.Nullable[api.SourcedText], kind string) nullable.Nullable[api.SourcedText] {
	if s, err := v.Get(); err == nil && g.ok(s, kind) {
		return v
	}
	return nullable.NewNullNullable[api.SourcedText]()
}

func isSet[T any](v nullable.Nullable[T]) bool {
	_, err := v.Get()
	return err == nil
}
