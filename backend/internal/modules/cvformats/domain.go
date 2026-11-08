package cvformats

// What fills a CV template, as plain functions on plain values: no database,
// no HTTP.

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

// Placeholders are the values a template may use.
var Placeholders = []string{
	"candidate.fullName", "candidate.firstName", "candidate.lastName", "candidate.initials", "candidate.reference",
	"candidate.currentTitle", "candidate.currentCompany", "candidate.location", "candidate.nationality",
	"candidate.workAuthorization", "candidate.noticePeriod", "candidate.email", "candidate.phone", "candidate.skills",
	"salary.current", "salary.expected", "summary", "job.title", "company.name",
	"agency.name", "recruiter.name", "recruiter.email", "date",
}

// BlockFields are the lists a template may repeat, and their fields.
var BlockFields = map[string][]string{
	"experience": {"title", "company", "dates", "description"},
	"education":  {"qualification", "institution", "year"},
}

// CheckTemplate says what is wrong with what a template uses, or "".
func CheckTemplate(f Found) string {
	var unknown []string
	for _, p := range f.Placeholders {
		if !slices.Contains(Placeholders, p) && !slices.Contains(unknown, p) {
			unknown = append(unknown, p)
		}
	}
	for block, names := range f.Blocks {
		cond, isCond := strings.CutSuffix(block, "?")
		fields, isList := BlockFields[cond]
		switch {
		case isCond && (isList || slices.Contains(Placeholders, cond)):
		case !isCond && isList:
		default:
			unknown = append(unknown, "#"+block)
			continue
		}
		for _, n := range names {
			itemField := !isCond && slices.Contains(fields, n)
			if !slices.Contains(Placeholders, n) && !itemField && !slices.Contains(unknown, n) {
				unknown = append(unknown, n)
			}
		}
	}
	if len(unknown) == 0 {
		return ""
	}
	slices.Sort(unknown)
	return "unknown placeholders: {{" + strings.Join(unknown, "}}, {{") + "}}"
}

// Job is one entry of the work history.
type Job struct {
	Title, Company     *string
	StartDate, EndDate *string // YYYY-MM or YYYY
	Current            bool
	Description        *string
}

// Qualification is one entry of the education.
type Qualification struct {
	Qualification, Institution *string
	EndYear                    *int
}

// Salary is a salary figure or range.
type Salary struct {
	Min      string
	Max      *string
	Currency string
	Period   string
}

// Candidate is what a CV shows about a candidate.
type Candidate struct {
	ID                                        string
	FirstName, LastName                       string
	Email, Phone, Nationality, Location       *string
	CurrentTitle, CurrentCompany              *string
	NoticeValue                               *int
	NoticeUnit                                string
	WorkAuthorization, WorkAuthorizationBasis *string
	Skills                                    []string
	Work                                      []Job
	Education                                 []Qualification
	CurrentSalary, ExpectedSalary             *Salary
}

// Context is everything else a CV shows.
type Context struct {
	ShowContact, IncludeSalary    bool
	Summary                       string
	JobTitle, CompanyName         string
	AgencyName                    string
	RecruiterName, RecruiterEmail string
	Today                         time.Time
}

func val(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

var months = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// monthYear writes 2020-03 as "Mar 2020" (and 2020 as is).
func monthYear(s *string) string {
	v := val(s)
	var y, m int
	if _, err := fmt.Sscanf(v, "%d-%d", &y, &m); err == nil && m >= 1 && m <= 12 {
		return fmt.Sprintf("%s %d", months[m-1], y)
	}
	return v
}

// Dates writes a job's dates: "Mar 2020 – Present", "2018 – 2020".
func Dates(j Job) string {
	start, end := monthYear(j.StartDate), monthYear(j.EndDate)
	if j.Current {
		end = "Present"
	}
	switch {
	case start != "" && start == end:
		return start // "2018 to 2018" says one year
	case start != "" && end != "":
		return start + " to " + end // as on the CVs sent to clients: "Jun 2022 to Present"
	case start != "":
		return "From " + start
	default:
		return end
	}
}

// Money writes a salary: "MYR 8,500.00 a month", "MYR 9,000 – 10,000 a month".
func Money(s *Salary) string {
	if s == nil {
		return ""
	}
	out := s.Currency + " " + grouped(s.Min)
	if s.Max != nil && *s.Max != "" && *s.Max != s.Min {
		out += " – " + grouped(*s.Max)
	}
	return out + " a " + s.Period
}

// grouped writes 8500.00 as 8,500.00 (and 9000 as 9,000).
func grouped(amount string) string {
	r, ok := new(big.Rat).SetString(amount)
	if !ok {
		return amount
	}
	whole, frac, _ := strings.Cut(amount, ".")
	if r.IsInt() && (frac == "" || strings.Trim(frac, "0") == "") {
		frac = ""
	}
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if frac != "" {
		return b.String() + "." + frac
	}
	return b.String()
}

var workAuthorization = map[string]string{
	"local": "Local", "expat_eligible": "Expatriate, can work here", "requires_visa": "Requires a work visa",
}
var workBasis = map[string]string{
	"rp_t": "Residence Pass-Talent", "permanent_resident": "permanent resident", "spouse_visa": "spouse visa", "other": "other pass",
}

// Values fills a template's data from a candidate. Contact details only when
// asked; salary only when asked (the caller checks the permission).
func Values(c Candidate, x Context) Data {
	full := strings.TrimSpace(c.FirstName + " " + c.LastName)
	initials := ""
	for _, part := range strings.Fields(full) {
		initials += strings.ToUpper(string([]rune(part)[0])) + "."
	}
	v := map[string]string{
		"candidate.fullName": full, "candidate.firstName": c.FirstName, "candidate.lastName": c.LastName,
		"candidate.initials": initials, "candidate.reference": Reference(c.ID), "candidate.currentTitle": SmartCase(val(c.CurrentTitle), false), "candidate.currentCompany": SmartCase(val(c.CurrentCompany), true),
		"candidate.location": val(c.Location), "candidate.nationality": val(c.Nationality),
		"candidate.skills": strings.Join(c.Skills, ", "), "summary": strings.TrimSpace(x.Summary),
		"job.title": x.JobTitle, "company.name": x.CompanyName, "agency.name": x.AgencyName,
		"recruiter.name": x.RecruiterName, "recruiter.email": x.RecruiterEmail, "date": x.Today.Format("2 January 2006"),
	}
	if c.WorkAuthorization != nil {
		v["candidate.workAuthorization"] = workAuthorization[*c.WorkAuthorization]
		if b := val(c.WorkAuthorizationBasis); b != "" {
			v["candidate.workAuthorization"] += " (" + workBasis[b] + ")"
		}
	}
	if c.NoticeValue != nil {
		switch n := *c.NoticeValue; n {
		case 0:
			v["candidate.noticePeriod"] = "Immediately"
		case 1:
			v["candidate.noticePeriod"] = "1 " + c.NoticeUnit
		default:
			v["candidate.noticePeriod"] = fmt.Sprintf("%d %ss", n, c.NoticeUnit)
		}
	}
	if x.ShowContact {
		v["candidate.email"], v["candidate.phone"] = val(c.Email), val(c.Phone)
	}
	if x.IncludeSalary {
		v["salary.current"], v["salary.expected"] = Money(c.CurrentSalary), Money(c.ExpectedSalary)
	}
	lists := map[string][]map[string]string{}
	for _, j := range c.Work {
		lists["experience"] = append(lists["experience"], map[string]string{
			"title": SmartCase(val(j.Title), false), "company": SmartCase(val(j.Company), true), "dates": Dates(j), "description": Duties(val(j.Description)),
		})
	}
	for _, q := range c.Education {
		year := ""
		if q.EndYear != nil {
			year = fmt.Sprint(*q.EndYear)
		}
		lists["education"] = append(lists["education"], map[string]string{
			"qualification": CleanQualification(val(q.Qualification)), "institution": SmartCase(val(q.Institution), true), "year": year,
		})
	}
	return Data{Values: v, Lists: lists}
}

// FileName names a formatted CV CV Studio's way: "Contoh Recruitment CV -
// Siti Aisyah.docx" (CV Studio: "Hyppies CV - Name.docx"); a blind one
// "Contoh Recruitment CV - Candidate C-4F2K9Q (Blinded).docx". CV Studio puts
// the real name in its blind file names too; here the reference stands in
// for it, so a file sent to a client does not name the candidate.
func FileName(c Candidate, agency string, blind bool) string {
	name, suffix := strings.TrimSpace(c.FirstName+" "+c.LastName), ""
	if blind {
		name, suffix = "Candidate "+Reference(c.ID), " (Blinded)"
	}
	prefix := "CV"
	if agency = strings.TrimSpace(agency); agency != "" {
		prefix = agency + " CV"
	}
	return prefix + " - " + name + suffix + ".docx"
}
