package settings

// The rules for dropdown lists and mandatory fields, as pure functions.

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Option is one choice in a dropdown.
type Option struct {
	Key, Label string
	Hint       string // what to write for it (lists with hints only)
	Color      string // e.g. #16a34a (lists with colours only)
	Hidden     bool
	BuiltIn    bool
}

// List is a dropdown with its options, in order.
type List struct {
	Name, Label string
	Fixed       bool // rename, reorder and hide only
	Hints       bool // its options carry a hint
	Colors      bool // its options carry a colour
	Options     []Option
}

// OptionInput is an option as a person saves it (Key empty for a new one).
type OptionInput struct {
	Key, Label, Hint, Color string
	Hidden                  bool
}

type listDef struct {
	name, label string
	fixed       bool
	hints       bool
	colors      bool
	defaults    []Option
}

// MaxHint is the longest hint an option may carry.
const MaxHint = 300

// withHints is opts with a hint for each option: key, label, hint, ...
func withHints(triples ...string) []Option {
	out := make([]Option, 0, len(triples)/3)
	for i := 0; i < len(triples); i += 3 {
		out = append(out, Option{Key: triples[i], Label: triples[i+1], Hint: triples[i+2], BuiltIn: true})
	}
	return out
}

func opts(pairs ...string) []Option {
	out := make([]Option, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Option{Key: pairs[i], Label: pairs[i+1], BuiltIn: true})
	}
	return out
}

// industries are Kano's industries, the ones the CV reader classifies into.
var industries = []string{
	"Digital & E-Commerce", "Financial Services", "FMCG", "Industrial/Manufacturing", "Information Technology & Services",
	"Life Science/Medical", "Property & Construction", "Professional Services", "Education", "Government Sector",
}

var lists = []listDef{
	{name: "candidate_source", label: "Candidate source", defaults: opts(
		"jobstreet", "JobStreet", "linkedin", "LinkedIn", "referral", "Referral", "careers_page", "Careers page",
		"direct", "Direct", "other", "Other")},
	{name: "notice_period", label: "Notice period", defaults: opts(
		"0_day", "Immediately", "1_week", "1 week", "2_week", "2 weeks", "1_month", "1 month", "2_month", "2 months", "3_month", "3 months")},
	{name: "industry", label: "Industry", defaults: func() []Option {
		var pairs []string
		for _, i := range industries {
			pairs = append(pairs, i, i)
		}
		return opts(pairs...)
	}()},
	{name: "work_type", label: "Work type", fixed: true, defaults: opts(
		"full_time", "Full time", "part_time", "Part time", "contract", "Contract", "temporary", "Temporary", "internship", "Internship")},
	{name: "work_arrangement", label: "Work arrangement", fixed: true, defaults: opts(
		"onsite", "On site", "hybrid", "Hybrid", "remote", "Remote")},
	// What to ask on a screening call, in order; each label is written before
	// its answer in the call's note ("RFL: better prospects"). Notice period
	// and salaries have their own fields on the call, so they are not items.
	{name: "screening_call", label: "Screening call", defaults: opts(
		"rfl", "RFL", "lf", "LF")},
	{name: "reference_check", label: "Reference check", defaults: opts(
		"referee", "Referee (name, title, company, contact)", "relationship", "How they worked together, and for how long",
		"role", "Role and responsibilities", "strengths", "Strengths", "improve", "Areas to improve",
		"reason_left", "Reason for leaving", "rehire", "Would you rehire them?")},
	// A client's benefits and its steps before an offer (the company page),
	// as in the Hyppies Benefits & Pre-Offer Process template.
	{name: "company_benefits", label: "Company benefits", hints: true, defaults: withHints(
		"job_grade", "Job grade/level", "",
		"annual_leave", "Annual leave", "If it grows with service or grade, give the detail",
		"other_leave", "Miscellaneous leave", "Compassionate, birthday, paternity/maternity: specify",
		"medical", "Medical and insurance", "Put * if extended to immediate family members, or say so in the remarks",
		"work_mode", "Work mode", "If hybrid, how many days in the office",
		"working_hours", "Working hours and flexibility", "If shifts are required, say whether there is a shift allowance",
		"dental_optical", "Dental & optical", "",
		"fixed_bonus", "Fixed bonus", "13th, 13.5 or 14th month; any specific clause",
		"performance_bonus", "Performance bonus", "Average, bonus cycles, payout",
		"other_bonus", "Miscellaneous bonus", "Gratuity, ESOS, retention",
		"commission", "Commission structure", "Payout cycle",
		"special", "Special benefits", "Free products, higher EPF, subsidies: anything really")},
	{name: "pre_offer_process", label: "Pre-offer process", hints: true, defaults: withHints(
		"reference_check", "Reference check", "How many, and which referees (superior/peers)",
		"background_check", "Background check", "",
		"health_screening", "Health screening", "",
		"payslips", "Payslips + bonus slips", "",
		"identity", "NRIC/passport (expat)", "",
		"education", "Highest education certificate", "Any specific certificate needed, like SPM",
		"additional", "Additional process", "Confirmation with HR, more documents to submit, registration for health screening...")},
	// A company's category, shown on its card in its colour.
	{name: "company_category", label: "Company categories", colors: true, defaults: []Option{
		{Key: "signed_client", Label: "Signed client", Color: "#16a34a", BuiltIn: true},
		{Key: "prospective_client", Label: "Prospective client", Color: "#2563eb", BuiltIn: true},
		{Key: "unknown", Label: "Unknown", Color: "#6b7280", BuiltIn: true},
		{Key: "vip_client", Label: "VIP client", Color: "#b8860b", BuiltIn: true},
	}},
}

// UnknownCategory is the category of a company without one.
const UnknownCategory = "unknown"

var colorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func def(name string) (listDef, bool) {
	for _, d := range lists {
		if d.name == name {
			return d, true
		}
	}
	return listDef{}, false
}

// Lists returns every dropdown: a company's saved options where it saved
// them, the built-in ones elsewhere.
func Lists(saved map[string][]Option) []List {
	out := make([]List, 0, len(lists))
	for _, d := range lists {
		options, ok := saved[d.name]
		if !ok {
			options = slices.Clone(d.defaults)
		}
		for i := range options {
			options[i].BuiltIn = slices.ContainsFunc(d.defaults, func(o Option) bool { return o.Key == options[i].Key })
		}
		out = append(out, List{Name: d.name, Label: d.label, Fixed: d.fixed, Hints: d.hints, Colors: d.colors, Options: options})
	}
	return out
}

var (
	noticeKey = regexp.MustCompile(`^(\d{1,3})_(day|week|month)$`)
	sourceKey = regexp.MustCompile(`^[a-z0-9_]{1,40}$`) // as a candidate's source
	keyChars  = regexp.MustCompile(`[^a-z0-9]+`)
)

// keyFrom makes a source key from a label: "Job Fair 2026" -> job_fair_2026.
func keyFrom(label string) string {
	k := strings.Trim(keyChars.ReplaceAllString(strings.ToLower(label), "_"), "_")
	if len(k) > 40 {
		k = strings.TrimRight(k[:40], "_")
	}
	return k
}

// CheckList checks a dropdown a person saves, and returns its options with
// keys: built-in options may be hidden but not removed; a fixed list keeps
// exactly its own options; labels are unique.
func CheckList(name string, in []OptionInput) ([]Option, error) {
	d, ok := def(name)
	if !ok {
		return nil, apperr.NotFound("list")
	}
	var errs apperr.FieldErrors
	out := make([]Option, 0, len(in))
	seenLabel := map[string]bool{}
	seenKey := map[string]bool{}
	for i, o := range in {
		field := fmt.Sprintf("options[%d]", i)
		label := strings.Join(strings.Fields(o.Label), " ")
		key := strings.TrimSpace(o.Key)
		switch {
		case name == "industry":
			key = label // an industry is stored as its words
		case name == "notice_period":
			m := noticeKey.FindStringSubmatch(key)
			if n, err := strconv.Atoi(firstOf(m)); m == nil || err != nil || n > 365 {
				errs.Add(field+".key", "a number of days, weeks or months, e.g. 2_week (at most 365)")
				continue
			}
		case key == "" && !d.fixed:
			key = keyFrom(label)
			for base, n := key, 2; seenKey[key]; n++ {
				key = fmt.Sprintf("%s_%d", base, n)
			}
		}
		if key == "" {
			errs.Add(field+".label", "use letters or numbers")
			continue
		}
		if name == "candidate_source" && !sourceKey.MatchString(key) {
			errs.Add(field+".key", "lower-case letters, numbers and _ (at most 40), or leave it out")
			continue
		}
		if label == "" {
			errs.Add(field+".label", "is required")
			continue
		}
		if seenLabel[strings.ToLower(label)] {
			errs.Add(field+".label", "is already in the list")
			continue
		}
		if seenKey[key] {
			errs.Add(field+".key", "is already in the list")
			continue
		}
		if d.fixed && !slices.ContainsFunc(d.defaults, func(b Option) bool { return b.Key == key }) {
			errs.Add(field+".key", "this list's options are fixed: rename or hide them")
			continue
		}
		hint := strings.Join(strings.Fields(o.Hint), " ")
		if !d.hints {
			hint = ""
		}
		if len([]rune(hint)) > MaxHint {
			errs.Add(field+".hint", fmt.Sprintf("must be at most %d characters", MaxHint))
			continue
		}
		color := strings.ToLower(strings.TrimSpace(o.Color))
		switch {
		case !d.colors:
			color = ""
		case color == "":
			color = "#6b7280" // grey until one is chosen (a built-in one keeps its own)
			if i := slices.IndexFunc(d.defaults, func(b Option) bool { return b.Key == key }); i >= 0 {
				color = d.defaults[i].Color
			}
		case !colorRe.MatchString(color):
			errs.Add(field+".color", "a colour such as #16a34a")
			continue
		}
		seenLabel[strings.ToLower(label)], seenKey[key] = true, true
		out = append(out, Option{Key: key, Label: label, Hint: hint, Color: color, Hidden: o.Hidden})
	}
	if name != "industry" { // industries are words: renaming one makes a new one
		for _, b := range d.defaults {
			if !seenKey[b.Key] {
				errs.Add("options", fmt.Sprintf("%q comes with the app: hide it instead of removing it", b.Label))
				break
			}
		}
	}
	return out, errs.Err()
}

func firstOf(m []string) string {
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// MandatoryField is a field a company can make mandatory on a form.
type MandatoryField struct {
	Form, Field, Label    string
	Mandatory, AlwaysReqd bool
}

type fieldDef struct {
	form, field, label string
	always             bool
}

var fields = []fieldDef{
	{"candidate", "firstName", "First name", true},
	{"candidate", "email", "Email", false},
	{"candidate", "phone", "Phone", false},
	{"candidate", "nationality", "Nationality", false},
	{"candidate", "residenceCountry", "Country of residence", false},
	{"candidate", "linkedinUrl", "LinkedIn profile", false},
	{"candidate", "location", "Location", false},
	{"candidate", "currentTitle", "Current title", false},
	{"candidate", "currentCompany", "Current company", false},
	{"candidate", "industry", "Industry", false},
	{"candidate", "source", "Source", false},
	{"candidate", "noticePeriod", "Notice period", false},
	{"candidate", "workAuthorization", "Work authorization", false},
	{"job", "title", "Title", true},
	{"job", "companyId", "Client company", true},
	{"job", "location", "Location", false},
	{"job", "country", "Country", false},
	{"job", "workType", "Work type", false},
	{"job", "workArrangement", "Work arrangement", false},
	{"job", "contactId", "Hiring contact", false},
	{"job", "description", "Description", false},
}

// Fields returns every field a form has, with whether it is mandatory
// (rules maps "form/field" to the company's choice).
func Fields(rules map[string]bool) []MandatoryField {
	out := make([]MandatoryField, 0, len(fields))
	for _, f := range fields {
		out = append(out, MandatoryField{Form: f.form, Field: f.field, Label: f.label, AlwaysReqd: f.always,
			Mandatory: f.always || rules[f.form+"/"+f.field]})
	}
	return out
}

// MandatoryOn returns the fields a company made mandatory on one form (not
// the ones the app always requires: the form's own rules check those).
func MandatoryOn(form string, rules map[string]bool) []string {
	var out []string
	for _, f := range fields {
		if f.form == form && !f.always && rules[f.form+"/"+f.field] {
			out = append(out, f.field)
		}
	}
	return out
}

// RuleInput is a person's choice for one field.
type RuleInput struct {
	Form, Field string
	Mandatory   bool
}

// CheckRules checks choices name real fields and leave always-required
// fields on.
func CheckRules(in []RuleInput) error {
	var errs apperr.FieldErrors
	for i, r := range in {
		j := slices.IndexFunc(fields, func(f fieldDef) bool { return f.form == r.Form && f.field == r.Field })
		switch {
		case j < 0:
			errs.Add(fmt.Sprintf("fields[%d].field", i), "is not a field of that form")
		case fields[j].always && !r.Mandatory:
			errs.Add(fmt.Sprintf("fields[%d].mandatory", i), "the app always needs this field")
		}
	}
	return errs.Err()
}
