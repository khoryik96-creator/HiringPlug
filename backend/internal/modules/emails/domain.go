package emails

// The email rules, as plain functions on plain values: no database, no HTTP.

import (
	"html"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Placeholders templates may use: every field's ({{candidate.expectedSalary}}
// is [Expected salary]) and the interview's details.
var Placeholders = func() []string {
	out := []string{"interview.title", "interview.length", "interview.how", "interview.instructions"}
	for _, f := range Fields {
		out = append(out, f.Key)
	}
	return out
}()

var placeholderRe = regexp.MustCompile(`\{\{\s*([a-zA-Z.]+)\s*\}\}`)

// Template is a reusable email: plain text, or a formatted message (BodyHTML,
// with its signature and banner) whose plain-text version is Body.
type Template struct{ Name, Subject, Body, BodyHTML string }

// Limits, in characters.
const (
	maxBody = 20000
	maxHTML = 100000
)

// NormalizeTemplate tidies a template, makes its formatted message safe (and
// its plain-text version from it), and refuses placeholders that do not exist.
func NormalizeTemplate(t Template) (Template, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	t.Name = strings.Join(strings.Fields(t.Name), " ")
	t.Subject = strings.Join(strings.Fields(t.Subject), " ")
	t.Body = strings.TrimSpace(t.Body)
	if t.BodyHTML != "" {
		t.BodyHTML = FieldMarksToTokens(SanitizeHTML(t.BodyHTML))
		t.Body = TextFromHTML(t.BodyHTML)
	}
	if t.Name == "" {
		errs.Add("name", "is required")
	}
	switch {
	case t.Body == "":
		errs.Add("body", "is required")
	case len([]rune(t.Body)) > maxBody:
		errs.Add("body", "is too long (at most 20,000 characters)")
	case len(t.BodyHTML) > maxHTML:
		errs.Add("bodyHtml", "is too long")
	}
	for field, text := range map[string]string{"subject": t.Subject, "body": t.Body} {
		for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
			if !slices.Contains(Placeholders, m[1]) {
				errs.Add(field, "{{"+m[1]+"}} is not a placeholder; use one of: "+strings.Join(Placeholders, ", "))
				break
			}
		}
	}
	return t, errs
}

var extraBlankLines = regexp.MustCompile(`\n{3,}`)

// Fill replaces placeholders with values. Those without a value (not in
// values) stay visible as [placeholder] and are listed, so they are fixed
// before sending; those whose value is empty on purpose (an interview without
// instructions) are left out, with the blank lines they leave.
func Fill(text string, values map[string]string) (string, []string) {
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := values[key]; ok {
			return v
		}
		if !slices.Contains(missing, key) {
			missing = append(missing, key)
		}
		return "[" + key + "]"
	})
	return extraBlankLines.ReplaceAllString(out, "\n\n"), missing
}

// FillHTML is Fill for a formatted message: the values are put in as text
// (escaped), never as markup.
func FillHTML(text string, values map[string]string) (string, []string) {
	escaped := make(map[string]string, len(values))
	for k, v := range values {
		escaped[k] = strings.ReplaceAll(html.EscapeString(v), "\n", "<br>")
	}
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := escaped[key]; ok {
			return v
		}
		if !slices.Contains(missing, key) {
			missing = append(missing, key)
		}
		return "[" + key + "]"
	})
	return out, missing
}

// ForClient reports whether a template is written to the client (its hiring
// contact) rather than to the candidate, by its name.
func ForClient(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "client") || strings.Contains(n, "submit")
}

// Starters are the templates every company begins with.
var Starters = []Template{
	{
		Name:    "Interview invitation",
		Subject: "Interview for {{job.title}} on {{interview.when}}",
		Body: "Dear {{candidate.firstName}},\n\nThank you for your interest in the {{job.title}} role. We would like to invite you to an interview:\n\n" +
			"When: {{interview.when}} ({{interview.length}})\nHow: {{interview.how}}\nWhere: {{interview.where}}\n\n{{interview.instructions}}\n\n" +
			"Please reply to confirm.\n\nBest regards,\n{{sender.name}}\n{{agency.name}}",
	},
	{
		Name:    "Submit candidate to client",
		Subject: "Candidate for {{job.title}}: {{candidate.fullName}}",
		Body: "Dear Hiring Manager,\n\nPlease find {{candidate.fullName}} for the {{job.title}} role at {{company.name}}. " +
			"Their CV is attached in our system; I am happy to arrange an interview.\n\nBest regards,\n{{sender.name}}\n{{agency.name}}",
	},
	{
		Name:    "Application received",
		Subject: "Your application for {{job.title}}",
		Body: "Dear {{candidate.firstName}},\n\nThank you for applying for {{job.title}}. We have received your application and will be in touch " +
			"if your profile matches the role.\n\nBest regards,\n{{sender.name}}\n{{agency.name}}",
	},
	{
		Name:    "Unsuccessful application",
		Subject: "Your application for {{job.title}}",
		Body: "Dear {{candidate.firstName}},\n\nThank you for your time and interest in the {{job.title}} role. After careful consideration, " +
			"we will not be moving forward with your application this time. We will keep your details for future roles that fit.\n\n" +
			"Best regards,\n{{sender.name}}\n{{agency.name}}",
	},
}

// Outgoing is an email to send: plain text, or a formatted message whose
// plain-text version is made from it.
type Outgoing struct {
	To, Cc, Bcc             []string
	Subject, Body, BodyHTML string
}

// NormalizeOutgoing tidies an email, makes its formatted message safe and
// checks its addresses.
func NormalizeOutgoing(o Outgoing) (Outgoing, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	clean := func(field string, list []string) []string {
		out := []string{}
		for _, a := range list {
			a = strings.ToLower(strings.TrimSpace(a))
			if a == "" || slices.Contains(out, a) {
				continue
			}
			if p, err := mail.ParseAddress(a); err != nil || p.Address != a {
				errs.Add(field, a+" is not a valid email address")
				continue
			}
			out = append(out, a)
		}
		return out
	}
	o.To, o.Cc, o.Bcc = clean("to", o.To), clean("cc", o.Cc), clean("bcc", o.Bcc)
	if len(o.To) == 0 {
		errs.Add("to", "add at least one recipient")
	}
	o.Subject = strings.Join(strings.Fields(o.Subject), " ") // no line breaks in a subject
	o.Body = strings.TrimSpace(o.Body)
	if o.BodyHTML != "" {
		o.BodyHTML = SanitizeHTML(o.BodyHTML)
		o.Body = TextFromHTML(o.BodyHTML)
	}
	if o.Subject == "" {
		errs.Add("subject", "is required")
	}
	if o.Body == "" && len(BannerIDs(o.BodyHTML)) == 0 {
		errs.Add("body", "is required")
	}
	return o, errs
}
