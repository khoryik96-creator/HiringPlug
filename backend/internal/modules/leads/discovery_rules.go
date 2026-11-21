package leads

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Keep aliases about job functions, so "worked with HR" does not add HR jobs
// to a search with a deliberate database or software engineering target.
var roleFamilies = [][]string{
	{"Quality Assurance", "QA engineer", "QA analyst", "quality engineer", "test automation", "software tester", "SDET", "software quality"},
	{"Database Administrator", "DBA", "database administration", "Oracle RAC", "RMAN", "Exadata", "database engineer"},
	{"SAP ERP", "SAP consultant", "SAP functional", "SAP technical", "ERP consultant", "ABAP"},
	{"Data Engineer", "data analyst", "data engineer", "data architect", "data scientist", "analytics engineer", "business intelligence"},
	{"Software Engineer", "software engineer", "software developer", "backend engineer", "frontend developer", "full stack developer"},
	{"Human Resources", "human resources", "HR business partner", "talent acquisition", "recruitment specialist", "people operations"},
	{"Finance", "financial analyst", "finance manager", "accountant", "accounting manager", "financial controller", "internal auditor"},
	{"Operations", "operations manager", "supply chain", "procurement specialist", "logistics manager"},
	{"Marketing", "digital marketing", "marketing manager", "brand manager", "performance marketing"},
	{"Sales", "account executive", "business development", "sales manager", "account manager"},
	{"Legal and Risk", "legal counsel", "legal officer", "risk analyst", "compliance officer"},
	{"Administration", "administrative assistant", "office manager", "executive assistant"},
	{"Product and Design", "product manager", "product designer", "UX designer", "UI designer"},
	{"Oil and Gas", "petroleum engineer", "drilling engineer", "reservoir engineer"},
	{"Consulting and Projects", "management consultant", "project manager", "program manager", "PMO analyst"},
}
var roleWords = regexp.MustCompile(`[a-z0-9]+`)
var modifiers = []string{"senior", "junior", "lead", "principal", "associate", "staff", "chief", "assistant", "head", "of", "the", "and"}

func roleQuery(cv, target string) (role, query string) {
	target = strings.Join(strings.Fields(target), " ")
	role = target
	// A CV can mention many industries. Read the opening role context and
	// require a recognisable title before broadening a search automatically.
	cv = strings.ToValidUTF8(cv[:min(len(cv), 3500)], "")
	text := " " + strings.Join(roleWords.FindAllString(strings.ToLower(cv), -1), " ") + " "
	if target != "" {
		text = " " + strings.Join(roleWords.FindAllString(strings.ToLower(target), -1), " ") + " "
	}
	best, family := 0, []string(nil)
	for _, options := range roleFamilies {
		score := 0
		for _, keyword := range options {
			if strings.Contains(text, " "+strings.ToLower(keyword)+" ") {
				score += len(keyword)
			}
		}
		if score > best {
			best, family = score, options
		}
	}
	if len(family) > 0 && role == "" {
		role = family[0]
	}
	if role == "" {
		return "", ""
	}
	parts := []string{}
	for _, phrase := range append([]string{role}, family...) {
		tokens := []string{}
		for _, word := range roleWords.FindAllString(strings.ToLower(phrase), 12) {
			if !slices.Contains(modifiers, word) && !slices.Contains(tokens, word) {
				tokens = append(tokens, word)
			}
		}
		if len(tokens) > 0 {
			part := "(" + strings.Join(tokens, " & ") + ")"
			if !slices.Contains(parts, part) {
				parts = append(parts, part)
			}
		}
	}
	return role, strings.Join(parts, " | ")
}

var regionTerms = map[string]string{
	"MY": `my|malaysia|kuala lumpur|selangor|petaling jaya|penang|pulau pinang|johor|cyberjaya|putrajaya|subang jaya|melaka|sabah|sarawak`,
	"SG": `sg|singapore`,
	"AU": `au|australia|sydney|melbourne|brisbane|perth|adelaide|canberra`,
	"NZ": `nz|new zealand|auckland|wellington|christchurch`,
	"GB": `gb|uk|united kingdom|england|scotland|wales|london|manchester|edinburgh`,
	"US": `us|usa|united states`,
	"HK": `hk|hong kong`,
	"ID": `id|indonesia|jakarta|surabaya|bandung`,
	"TH": `th|thailand|bangkok`,
	"VN": `vn|vietnam|viet nam|hanoi|ho chi minh`,
	"PH": `ph|philippines|manila|makati|cebu`,
	"IN": `india|bengaluru|bangalore|mumbai|delhi|hyderabad|pune`,
}

func locationPattern(countries []string) string {
	terms := make([]string, 0, len(countries))
	for _, country := range countries {
		if v := regionTerms[country]; v != "" {
			terms = append(terms, v)
		}
	}
	if len(terms) == 0 {
		return ""
	}
	return `(^|[^[:alnum:]])(` + strings.Join(terms, "|") + `)([^[:alnum:]]|$)`
}

var directID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,179}$`)
var greenhouseID = regexp.MustCompile(`^[0-9]+$`)

func directFeedAd(kind, raw, openingID string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawPath != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	var valid bool
	switch kind {
	case Greenhouse:
		valid = (host == "boards.greenhouse.io" || host == "job-boards.greenhouse.io" || host == "job-boards.eu.greenhouse.io") &&
			len(p) == 3 && directID.MatchString(p[0]) && p[1] == "jobs" && greenhouseID.MatchString(p[2])
		if valid && host == "boards.greenhouse.io" {
			host = "job-boards.greenhouse.io"
		}
	case Lever:
		valid = (host == "jobs.lever.co" || host == "jobs.eu.lever.co") && len(p) == 2 &&
			directID.MatchString(p[0]) && directID.MatchString(p[1])
	case Ashby:
		valid = host == "jobs.ashbyhq.com" && len(p) == 2 && directID.MatchString(p[0]) && directID.MatchString(p[1])
	case SmartRecruiters:
		valid = host == "jobs.smartrecruiters.com" && len(p) == 2 && directID.MatchString(p[0]) && directID.MatchString(p[1])
	}
	if !valid || p[len(p)-1] != openingID {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: host, Path: "/" + strings.Join(p, "/")}).String()
}
