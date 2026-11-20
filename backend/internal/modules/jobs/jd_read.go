package jobs

// Reading a client's job description to fill in an ad: pure rules, no
// database. Labelled lines ("Location: Kuala Lumpur") win; otherwise the
// title is the first short line, and the rest is found in the text (a salary
// such as "RM 8,000 - 10,000 per month", "hybrid", "contract", a city).

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// JDDetails are an ad's details read from a job description ("" or nil when it does not say).
type JDDetails struct {
	Title, Location, Country, WorkType, WorkArrangement, SalaryText string
	ClosesOn                                                        *time.Time
	Description                                                     string
	CompanyName                                                     string // the hiring company it names
	Openings                                                        *int   // how many are wanted
}

var (
	bulletRe   = regexp.MustCompile(`^\s*(?:[•●▪◦○■□➢➤►▶✓✔·*–—-]|\(?[0-9]{1,2}[.)]|\(?[a-hA-H][.)])\s+`)
	labelRe    = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z /&.]{1,30}?)\s*[:：]\s*(.*)$`)
	spaceRe    = regexp.MustCompile(`[ \t\x{00a0}]+`)
	headingRe  = regexp.MustCompile(`(?i)^(job description|jd|position description|about (us|the (company|role|job))|overview|summary|(key |job |main )?(responsibilities|requirements|qualifications|duties)|what you will do|who you are|what we offer|benefits|the role|role)\s*:?$`)
	salaryRe   = regexp.MustCompile(`(?i)(?:RM|MYR|SGD|S\$|USD|US\$)\s?\d[\d,]*(?:\.\d+)?\s?[kK]?(?:\s?(?:-|–|to)\s?(?:RM|MYR|SGD|S\$|USD|US\$)?\s?\d[\d,]*(?:\.\d+)?\s?[kK]?)?(?:\s?(?:per|/|a|an)\s?(?:month|mth|annum|year|yr|hour|hr))?`)
	dateDMYRe  = regexp.MustCompile(`\b(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})\b`)
	dateISORe  = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	contractRe = regexp.MustCompile(`\bcontract(ual|or)?\b`)
	remoteRe   = regexp.MustCompile(`\b(fully remote|remote work|work from home|wfh|remote)\b`)
	onsiteRe   = regexp.MustCompile(`\b(on-site|onsite|on site|in office|office-based|office based)\b`)
	dateWordRe = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]{3,9})\.?,?\s+(\d{4})\b|\b([A-Za-z]{3,9})\.?\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})\b`)
)

// Labels a description uses, by the ad detail they give.
var jdLabels = map[string][]string{
	"title":    {"job title", "position", "position title", "role", "title", "job", "designation", "vacancy"},
	"location": {"location", "work location", "job location", "based in", "office", "workplace", "place of work"},
	"salary":   {"salary", "salary range", "remuneration", "compensation", "pay", "monthly salary", "package", "salary package"},
	"type":     {"employment type", "job type", "type", "work type", "employment", "contract type"},
	"arrange":  {"work arrangement", "arrangement", "work mode", "working arrangement", "work model"},
	"closes":   {"closing date", "apply by", "application deadline", "deadline", "closes", "closing"},
	"company":  {"company", "company name", "employer", "hiring company", "organisation", "organization", "client", "syarikat"},
	"openings": {"no. of vacancies", "no of vacancies", "vacancies", "openings", "headcount", "head count", "no. of positions", "number of positions", "no. of headcount", "positions available"},
}

var (
	// legalNameRe is a company's registered name: "Maju Foods Berhad", "Contoh (M) Sdn. Bhd.".
	legalNameRe = regexp.MustCompile(`(?i)^[\p{L}\p{N}&.,'()\- ]{2,80}?\b(?:sdn\.?\s*bhd\.?|berhad|bhd\.?|pte\.?\s*ltd\.?|ltd\.?|limited|inc\.?|plc|llc|corporation|corp\.?|group)\s*$`)
	aboutRe     = regexp.MustCompile(`(?i)^about\s+(.{2,80}?)\s*:?$`)
	countRe     = regexp.MustCompile(`\b([1-9]\d{0,2})\b`)
	openingsRe  = regexp.MustCompile(`(?i)\b([1-9]\d?)\s+(?:positions|vacancies|openings|headcount|pax)\b|\(\s*x\s*([1-9]\d?)\s*\)|\b(?:hiring|recruiting)\s+([1-9]\d?)\s+[a-z]`)
	// roleRe is a word job titles have: a line with one is the title before a slogan or a heading.
	roleRe = regexp.MustCompile(`(?i)\b(?:engineers?|managers?|analysts?|executives?|officers?|specialists?|consultants?|directors?|lead|head|heads|assistants?|accountants?|developers?|designers?|planners?|architects?|administrators?|coordinators?|supervisors?|technicians?|associates?|advisors?|auditors?|controllers?|scientists?|representatives?|clerks?|interns?|trainees?|vp|chief|president|partner|programmer|tester|operator|buyer|merchandiser|nurse|pharmacist|doctor|lecturer|teacher|chef|driver|cashier|receptionist|secretary)\b`)
)

// Places a description may name, with their country.
var jdPlaces = []struct{ name, country string }{
	{"Kuala Lumpur", "MY"}, {"Petaling Jaya", "MY"}, {"Shah Alam", "MY"}, {"Subang Jaya", "MY"}, {"Cyberjaya", "MY"},
	{"Putrajaya", "MY"}, {"Puchong", "MY"}, {"Klang", "MY"}, {"Bangsar", "MY"}, {"Mont Kiara", "MY"}, {"Damansara", "MY"},
	{"Penang", "MY"}, {"George Town", "MY"}, {"Bayan Lepas", "MY"}, {"Johor Bahru", "MY"}, {"Ipoh", "MY"}, {"Melaka", "MY"},
	{"Seremban", "MY"}, {"Kota Kinabalu", "MY"}, {"Kuching", "MY"}, {"Selangor", "MY"}, {"Johor", "MY"},
	{"Singapore", "SG"}, {"Jakarta", "ID"}, {"Bangkok", "TH"}, {"Manila", "PH"}, {"Ho Chi Minh City", "VN"}, {"Hanoi", "VN"},
	{"Hong Kong", "HK"}, {"Malaysia", "MY"}, {"Indonesia", "ID"}, {"Thailand", "TH"}, {"Philippines", "PH"}, {"Vietnam", "VN"},
}

var months = map[string]time.Month{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// ReadJD reads an ad's details from a job description's text. A closing
// date is kept only when it is after today.
func ReadJD(text string, today time.Time) JDDetails {
	var d JDDetails
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(spaceRe.ReplaceAllString(raw, " "))
		if m := bulletRe.FindString(line); m != "" && len(line) > len(m) {
			line = "- " + strings.TrimSpace(line[len(m):])
		}
		lines = append(lines, line)
	}

	used := map[int]bool{} // labelled lines given to a detail, left out of the ad text
	for i, line := range lines {
		m := labelRe.FindStringSubmatch(strings.TrimPrefix(line, "- "))
		if m == nil || strings.TrimSpace(m[2]) == "" {
			continue
		}
		label, value := strings.ToLower(strings.TrimSpace(m[1])), strings.TrimSpace(m[2])
		switch labelFor(label) {
		case "title":
			if d.Title == "" {
				d.Title, used[i] = clipRunes(value, 200), true
			}
		case "location":
			if d.Location == "" {
				d.Location, used[i] = clipRunes(value, 200), true
			}
		case "salary":
			if d.SalaryText == "" {
				// Just the amount when the line says more ("Pay: S$8,000/month. Apply by..."), and the line stays in the text.
				amount := strings.TrimSpace(salaryRe.FindString(value))
				if amount == "" || strings.TrimRight(value, " .;") == amount {
					d.SalaryText, used[i] = clipRunes(strings.TrimRight(value, " .;"), 200), true
				} else {
					d.SalaryText = amount
				}
			}
		case "type":
			if t := workTypeIn(value); t != "" && d.WorkType == "" {
				d.WorkType, used[i] = t, true
			}
		case "arrange":
			if a := arrangementIn(value); a != "" && d.WorkArrangement == "" {
				d.WorkArrangement, used[i] = a, true
			}
		case "company":
			if d.CompanyName == "" && !strings.ContainsAny(value, ".;") {
				d.CompanyName, used[i] = clipRunes(value, 200), true
			}
		case "openings":
			if m := countRe.FindStringSubmatch(value); m != nil && d.Openings == nil {
				n := atoi(m[1])
				d.Openings, used[i] = &n, true
			}
		case "closes":
			if day := dateIn(value); day != nil && d.ClosesOn == nil {
				used[i] = true
				if day.After(today) {
					d.ClosesOn = day
				}
			}
		}
	}

	if d.CompanyName == "" {
		d.CompanyName = companyIn(lines)
	}
	if d.Title == "" {
		for i, line := range lines {
			if line == "" || strings.HasPrefix(line, "- ") || headingRe.MatchString(line) {
				continue
			}
			if legalNameRe.MatchString(line) { // the client's letterhead, not the title
				continue
			}
			if words := len(strings.Fields(line)); words <= 10 && len([]rune(line)) <= 90 && !strings.ContainsAny(line, ":.;") {
				d.Title, used[i] = strings.TrimSpace(strings.TrimPrefix(line, "Job Description - ")), true
			}
			break
		}
		// A slogan or a heading came first ("Banking Made Simple"): the title is the
		// first short line with a job-title word, when there is one.
		if !roleRe.MatchString(d.Title) {
			for i, line := range lines {
				short := len(strings.Fields(line)) <= 8 && len([]rune(line)) <= 80 && !strings.ContainsAny(line, ":.;")
				if line != "" && short && !strings.HasPrefix(line, "- ") && !legalNameRe.MatchString(line) && roleRe.MatchString(line) {
					for j := range used {
						if lines[j] == d.Title {
							delete(used, j)
						}
					}
					d.Title, used[i] = strings.TrimSpace(strings.TrimPrefix(line, "Job Description - ")), true
					break
				}
			}
		}
	}
	whole := strings.Join(lines, "\n")
	if d.SalaryText == "" {
		d.SalaryText = strings.TrimSpace(salaryRe.FindString(whole))
	}
	if d.WorkType == "" {
		d.WorkType = workTypeIn(whole)
	}
	if d.WorkArrangement == "" {
		d.WorkArrangement = arrangementIn(whole)
	}
	if d.Location == "" {
		d.Location = placeIn(whole)
	}
	if d.Openings == nil {
		if m := openingsRe.FindStringSubmatch(whole); m != nil {
			n := atoi(m[1] + m[2] + m[3])
			d.Openings = &n
		}
	}
	d.Country = countryOf(d.Location)
	if d.Country == "" {
		d.Country = countryOf(placeIn(whole))
	}

	var body []string
	for i, line := range lines {
		if used[i] || (line == "" && (len(body) == 0 || body[len(body)-1] == "")) {
			continue
		}
		body = append(body, line)
	}
	d.Description = clipRunes(strings.TrimSpace(strings.Join(unwrap(body), "\n")), 20000)
	return d
}

// companyIn is the hiring company a description names: its registered name
// on a line of its own near the top (a letterhead), or after "About".
func companyIn(lines []string) string {
	seen := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		if m := aboutRe.FindStringSubmatch(line); m != nil && !headingRe.MatchString(line) && isHeading(line) {
			return clipRunes(strings.TrimSpace(m[1]), 200)
		}
		if seen < 6 && legalNameRe.MatchString(line) {
			return clipRunes(strings.TrimSpace(line), 200)
		}
		seen++
	}
	return ""
}

// unwrap undoes a PDF's line wrapping (a line going on in lower case joins
// the one before, bullet points included) and leaves a blank line before
// each heading ("Requirements"), so the ad reads as paragraphs.
func unwrap(lines []string) []string {
	var out []string
	for _, line := range lines {
		last := len(out) - 1
		if line != "" && last >= 0 && out[last] != "" && !strings.HasPrefix(line, "- ") && startsLower(line) && !endsSentence(out[last]) {
			out[last] += " " + line
			continue
		}
		if isHeading(line) && last >= 0 && out[last] != "" {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return out
}

func startsLower(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

func endsSentence(s string) bool {
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, ":") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?")
}

// isHeading: a known heading, or a short line in title case with no full stop ("About Maju Foods Berhad").
func isHeading(s string) bool {
	if headingRe.MatchString(s) {
		return true
	}
	words := strings.Fields(s)
	if s == "" || strings.HasPrefix(s, "- ") || len(words) > 6 || strings.ContainsAny(s, ".,;") {
		return false
	}
	for _, w := range words {
		if r := []rune(w)[0]; unicode.IsLetter(r) && !unicode.IsUpper(r) && len(w) > 3 {
			return false
		}
	}
	return true
}

func labelFor(label string) string {
	for detail, names := range jdLabels {
		for _, n := range names {
			if label == n {
				return detail
			}
		}
	}
	return ""
}

func workTypeIn(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "intern"):
		return "internship"
	case strings.Contains(l, "part-time") || strings.Contains(l, "part time"):
		return "part_time"
	case strings.Contains(l, "temporary") || strings.Contains(l, "temp role"):
		return "temporary"
	case contractRe.MatchString(l) && !strings.Contains(l, "permanent"):
		return "contract"
	case strings.Contains(l, "full-time") || strings.Contains(l, "full time") || strings.Contains(l, "permanent"):
		return "full_time"
	}
	return ""
}

func arrangementIn(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "hybrid"):
		return "hybrid"
	case remoteRe.MatchString(l):
		return "remote"
	case onsiteRe.MatchString(l):
		return "onsite"
	}
	return ""
}

func placeIn(s string) string {
	l := strings.ToLower(s)
	best, at := "", -1
	for _, p := range jdPlaces {
		if i := strings.Index(l, strings.ToLower(p.name)); i >= 0 && (at < 0 || i < at) {
			best, at = p.name, i
		}
	}
	return best
}

func countryOf(location string) string {
	l := strings.ToLower(location)
	for _, p := range jdPlaces {
		if l != "" && strings.Contains(l, strings.ToLower(p.name)) {
			return p.country
		}
	}
	return ""
}

func dateIn(s string) *time.Time {
	day := func(y, m, d int) *time.Time {
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		if t.Month() != time.Month(m) || t.Day() != d {
			return nil
		}
		return &t
	}
	if m := dateISORe.FindStringSubmatch(s); m != nil {
		return day(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := dateDMYRe.FindStringSubmatch(s); m != nil {
		return day(atoi(m[3]), atoi(m[2]), atoi(m[1])) // day first, as written in Malaysia and Singapore
	}
	if m := dateWordRe.FindStringSubmatch(s); m != nil {
		if m[1] != "" {
			if mon, ok := monthOf(m[2]); ok {
				return day(atoi(m[3]), int(mon), atoi(m[1]))
			}
		} else if mon, ok := monthOf(m[4]); ok {
			return day(atoi(m[6]), int(mon), atoi(m[5]))
		}
	}
	return nil
}

func monthOf(name string) (time.Month, bool) {
	l := strings.ToLower(name)
	if len(l) > 3 {
		l = l[:3]
	}
	m, ok := months[l]
	return m, ok
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
