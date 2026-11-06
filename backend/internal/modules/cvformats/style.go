package cvformats

// House style for formatted CVs, from CV Studio's formatting rules
// (cvstudio_cv_normalize.py and generate.js): duties as one bullet each with
// the markers the source typed taken out, names typed in capitals put in
// title case (acronyms and brands kept), placeholder degrees and job-board
// tracking lines removed. Pure functions: what is shown changes, never the
// candidate's record.

import (
	"regexp"
	"strings"
	"unicode"
)

// leadingMarker is a list marker a source line carries in its own text:
// glyph bullets always; "(i)", "(a)", "(12)"; "1." or "1)" (not "3.5");
// "1- "; lower-case "a." "ii." "a-" before a space; a dash or asterisk
// before a space or a letter (not "-5%").
var leadingMarker = regexp.MustCompile(`^\s*(?:` +
	`[•●▪◦‣∙·▶►➤⁃»›]` +
	`|\((?:[ivxlcdmIVXLCDM]{1,7}|[A-Za-z]|\d{1,3})\)` +
	`|\d{1,3}[.)](?:\s|\pL)` +
	`|\d{1,3}-\s` +
	`|(?:[a-z]|[ivxlcdm]{2,7})[.)-]\s` +
	`|[*‐‑‒–—―-](?:\s|\pL)` +
	`)`)

// stripMarker takes off leading list markers (a short run such as "• - ").
func stripMarker(line string) string {
	for range 5 {
		m := leadingMarker.FindString(line)
		if m == "" {
			break
		}
		// The lookahead character (a letter) belongs to the text, not the marker.
		if last := []rune(m)[len([]rune(m))-1]; unicode.IsLetter(last) {
			m = string([]rune(m)[:len([]rune(m))-1])
		}
		line = line[len(m):]
	}
	return strings.TrimSpace(line)
}

// tracking is job-board application metadata (JobStreet's SiVA folders),
// not part of a CV.
var tracking = regexp.MustCompile(`(?i)position\s*:\s*retrieved\s+resumes\s*\(\s*siva\s+folder\s*:[^)\n]*\)\s*;\s*date\s+applied\s*:[^\n]*`)

// Duties is a job's description as one duty per line: markers the source
// typed taken out, marker-only and empty lines dropped, the first letter
// capitalised, repeated lines once.
func Duties(description string) string {
	var out []string
	for _, raw := range strings.Split(tracking.ReplaceAllString(description, ""), "\n") {
		line := stripMarker(strings.Join(strings.Fields(raw), " "))
		if !strings.ContainsFunc(line, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue // "-", "--", "•": residue of a marker
		}
		r := []rune(line)
		r[0] = unicode.ToUpper(r[0])
		line = string(r)
		if len(out) > 0 && out[len(out)-1] == line {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var (
	companyTokens = map[string]string{
		"SDN": "Sdn", "BHD": "Bhd", "PTE": "Pte", "LTD": "Ltd", "LMT": "Lmt", "PVT": "Pvt", "INC": "Inc",
		"LLC": "LLC", "LLP": "LLP", "PLC": "PLC", "CORP": "Corp", "CO": "Co", "COMPANY": "Company", "TECH": "Tech",
	}
	brandTokens = map[string]string{"IFAST": "iFAST", "GRABPAY": "GrabPay", "DATAIKU": "Dataiku", "YOUTUBE": "YouTube"}
	titleTokens = map[string]string{"SR": "Sr", "SR.": "Sr.", "JR": "Jr", "JR.": "Jr.", "VP": "VP", "AVP": "AVP"}
	acronyms    = map[string]bool{}
	smallWords  = map[string]bool{"of": true, "and": true, "the": true, "for": true, "in": true, "at": true, "to": true, "on": true, "&": true}
	splitParts  = regexp.MustCompile(`(\s+|/|\||,|;|\(|\)|\[|\])`)
	tokenCore   = regexp.MustCompile(`[^A-Za-z0-9&.+#]`)
	asciiWord   = regexp.MustCompile(`[A-Za-z]+`)
)

func init() {
	for _, a := range strings.Fields("AI ML BI IT HR QA UA UX UI PMO PM AWS GCP SQL ETL ELT SSIS SSRS SSAS ADF DBA RDS EMR EC2 S3 IAM API " +
		"APAC SEA ERP SAP FICO MSBI MSC IBM CGI EPAM TCS HP HSBC DBS OCBC UOB AIA IHH RHB CIMB EY KPMG PWC BNM AML") {
		acronyms[a] = true
	}
}

// SmartCase puts a company name or job title typed mostly in capitals
// ("LENOVO PCCW SOLUTIONS SDN BHD") in title case, keeping acronyms ("PCCW",
// "TDCX", "IBM") and brands ("iFAST"). Mixed-case text is kept, except
// company suffixes in capitals ("MSC SDN BHD" -> "MSC Sdn Bhd") and lower-case
// company words.
func SmartCase(text string, company bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	letters, upper := 0, 0
	for _, r := range text {
		if r < unicode.MaxASCII && unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	mostlyCaps := letters > 0 && float64(upper)/float64(letters) >= 0.72
	parts := splitParts.Split(text, -1)
	seps := splitParts.FindAllString(text, -1)
	var b strings.Builder
	for i, part := range parts {
		switch {
		case part == "":
		case mostlyCaps:
			b.WriteString(wordCase(part, company, i > 0))
		case company:
			core := tokenCore.ReplaceAllString(part, "")
			up := strings.ToUpper(core)
			_, known := companyTokens[up]
			_, brand := brandTokens[up]
			if known || brand || acronyms[up] || (len(core) > 2 && isAlpha(core) && core == strings.ToLower(core) && !smallWords[core]) {
				b.WriteString(wordCase(part, company, i > 0))
			} else {
				b.WriteString(part)
			}
		default:
			b.WriteString(part)
		}
		if i < len(seps) {
			b.WriteString(seps[i])
		}
	}
	return strings.TrimSpace(b.String())
}

func wordCase(token string, company, inner bool) string {
	core := tokenCore.ReplaceAllString(token, "")
	if core == "" {
		return token
	}
	up := strings.ToUpper(core)
	var repl string
	switch {
	case company && brandTokens[up] != "":
		repl = brandTokens[up]
	case company && companyTokens[up] != "":
		repl = companyTokens[up]
	case !company && titleTokens[up] != "":
		repl = titleTokens[up]
	case acronyms[up], isAlpha(core) && core == up && len(core) <= 3 && (!inner || !smallWords[strings.ToLower(core)]),
		isAlpha(core) && core == up && len(core) >= 2 && !strings.ContainsAny(up, "AEIOU"):
		repl = up
	case inner && smallWords[strings.ToLower(core)]:
		return strings.ToLower(token)
	default:
		return asciiWord.ReplaceAllStringFunc(strings.ToLower(token), func(w string) string {
			return strings.ToUpper(w[:1]) + w[1:]
		})
	}
	if i := strings.Index(token, core); i >= 0 {
		return token[:i] + repl + token[i+len(core):]
	}
	return repl
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

var (
	emptyDegree    = regexp.MustCompile(`(?i)^(?:no\s+degree|degree\s+(?:not\s+)?(?:specified|stated)|not\s+(?:specified|stated)|n\s*/?\s*a|none|unknown)$`)
	noDegreePrefix = regexp.MustCompile(`(?i)^no\s+degree\s*(?::|[-–—])\s*(.+?)\s*$`)
)

// CleanQualification drops placeholders such as "No Degree" or "N/A" ("No
// Degree: Diploma in IT" keeps "Diploma in IT").
func CleanQualification(q string) string {
	q = strings.TrimSpace(q)
	if m := noDegreePrefix.FindStringSubmatch(q); m != nil {
		return m[1]
	}
	if emptyDegree.MatchString(q) {
		return ""
	}
	return q
}
