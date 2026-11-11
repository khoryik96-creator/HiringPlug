package emails

// Fields: what a person types or inserts in a message ("[Expected salary]")
// and the placeholder it stands for (candidate.expectedSalary). Older
// templates use {{candidate.firstName}}; both are filled the same way.

import (
	"html"
	"regexp"
	"slices"
	"strings"
)

// Field is one fillable value.
type Field struct {
	Label  string // shown in brackets: [Expected salary]
	Key    string // the placeholder: candidate.expectedSalary
	Salary bool   // fills only for senders who may see salaries
}

// Token is how the field is written in a message.
func (f Field) Token() string { return "[" + f.Label + "]" }

// Fields are the fields a message may use, in the order they are offered.
var Fields = []Field{
	{Label: "First name", Key: "candidate.firstName"},
	{Label: "Last name", Key: "candidate.lastName"},
	{Label: "Full name", Key: "candidate.fullName"},
	{Label: "Current salary", Key: "candidate.currentSalary", Salary: true},
	{Label: "Expected salary", Key: "candidate.expectedSalary", Salary: true},
	{Label: "Salary notes", Key: "candidate.salaryNotes", Salary: true},
	{Label: "Current salary breakdown", Key: "candidate.currentBreakdown", Salary: true},
	{Label: "Expected salary notes", Key: "candidate.expectedNotes", Salary: true},
	{Label: "Industries", Key: "candidate.industries"},
	{Label: "Notice period", Key: "candidate.noticePeriod"},
	{Label: "Expat status", Key: "candidate.workAuthorization"},
	{Label: "Nationality", Key: "candidate.nationality"},
	{Label: "Lives in", Key: "candidate.residence"},
	{Label: "Current title", Key: "candidate.currentTitle"},
	{Label: "Current company", Key: "candidate.currentCompany"},
	{Label: "Job title", Key: "job.title"},
	{Label: "Client company", Key: "company.name"},
	{Label: "Client contact", Key: "client.name"},
	{Label: "Interview time", Key: "interview.when"},
	{Label: "Interview place", Key: "interview.where"},
	{Label: "Your name", Key: "sender.name"},
	{Label: "Agency", Key: "agency.name"},
}

// tokenRe finds [Words] that may be a field; text in brackets that names no
// field ("[optional]") is left as it is.
var tokenRe = regexp.MustCompile(`\[([A-Za-z][A-Za-z ]{1,30})\]`)

func fieldByLabel(label string) (Field, bool) {
	i := slices.IndexFunc(Fields, func(f Field) bool { return strings.EqualFold(f.Label, strings.TrimSpace(label)) })
	if i < 0 {
		return Field{}, false
	}
	return Fields[i], true
}

func fieldByKey(key string) (Field, bool) {
	i := slices.IndexFunc(Fields, func(f Field) bool { return f.Key == key })
	if i < 0 {
		return Field{}, false
	}
	return Fields[i], true
}

// Filled is a message with its fields filled.
type Filled struct {
	Text    string
	Missing []string // fields with no value, as tokens ("[Expected salary]"), left in the text
	Salary  bool     // a salary field was filled
}

// FillFields fills [Field] tokens and {{placeholders}} from values. With
// asHTML the values are escaped for HTML. A field with no value stays as its
// token and is listed in Missing.
func FillFields(text string, values map[string]string, asHTML bool) Filled {
	var out Filled
	value := func(key, token string) string {
		v, ok := values[key]
		if !ok {
			if !slices.Contains(out.Missing, token) {
				out.Missing = append(out.Missing, token)
			}
			return token
		}
		if f, _ := fieldByKey(key); f.Salary {
			out.Salary = true
		}
		if asHTML {
			return strings.ReplaceAll(html.EscapeString(v), "\n", "<br>")
		}
		return v
	}
	text = tokenRe.ReplaceAllStringFunc(text, func(m string) string {
		f, ok := fieldByLabel(tokenRe.FindStringSubmatch(m)[1])
		if !ok {
			return m
		}
		return value(f.Key, f.Token())
	})
	out.Text = placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		token := "{{" + key + "}}"
		if f, ok := fieldByKey(key); ok {
			token = f.Token()
		}
		return value(key, token)
	})
	return out
}
