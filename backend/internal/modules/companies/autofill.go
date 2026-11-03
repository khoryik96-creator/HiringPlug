package companies

// Reading a company's own website for details to suggest, as pure functions
// on the page's HTML: schema.org Organization data (JSON-LD, the markup sites
// add for search engines), the page's description, phone links, and a
// Malaysian company registration number written on the page.

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Suggestion is a detail found on a website, and where.
type Suggestion struct{ Field, Value, Source string }

var (
	ldJSON      = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	metaTag     = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	attr        = regexp.MustCompile(`(?is)\b(name|property|content)\s*=\s*("([^"]*)"|'([^']*)')`)
	telLink     = regexp.MustCompile(`(?i)href\s*=\s*["']tel:([^"']+)["']`)
	scripts     = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	tags        = regexp.MustCompile(`(?s)<[^>]+>`)
	spaces      = regexp.MustCompile(`\s+`)
	contactHref = regexp.MustCompile(`(?i)<a\s[^>]*href\s*=\s*["']([^"']*(?:contact|hubungi)[^"']*)["']`)
	// "Company No. 1234567-X", "Registration No: 201901012345 (1234567-X)", "SSM: 1234567-X".
	registration = regexp.MustCompile(`(?i)(?:company\s*(?:no|number|reg(?:istration)?\.?\s*no)|registration\s*(?:no|number)|reg\.?\s*no|ssm)\.?\s*[:#]?\s*((?:\d{12}\s*)?\(?\d{5,8}-[A-Z]\)?|\d{12})`)
	// "Contoh Sdn Bhd (1234567-X)" without a label.
	registrationAfterName = regexp.MustCompile(`(?i)(?:sdn\.?\s*bhd\.?|berhad|plt)\s*\(\s*((?:\d{12}\s*\()?\d{5,8}-[A-Z]\)?)\s*\)?`)
)

// ReadSite reads suggestions from one page.
func ReadSite(page []byte, pageURL string) []Suggestion {
	src := func(what string) string { return clip(what+" on "+pageURL, 500) }
	var out []Suggestion
	add := func(field, value, source string) {
		value = clip(strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(value), " ")), 4000)
		if value == "" {
			return
		}
		for _, s := range out {
			if s.Field == field {
				return // the first source wins
			}
		}
		out = append(out, Suggestion{Field: field, Value: value, Source: src(source)})
	}

	if org := organization(page, pageURL); org != nil {
		add("name", firstText(org["legalName"], org["name"]), "schema.org data")
		add("phone", firstText(org["telephone"]), "schema.org data")
		if a, ok := first(org["address"]).(map[string]any); ok {
			street := firstText(a["streetAddress"])
			if pc := firstText(a["postalCode"]); pc != "" && !strings.Contains(street, pc) {
				street = strings.TrimSpace(street + ", " + pc)
			}
			add("address", street, "schema.org data")
			add("city", firstText(a["addressLocality"]), "schema.org data")
			if code := CountryCode(firstText(a["addressCountry"])); code != "" {
				add("country", code, "schema.org data")
			}
		}
		if size := SizeOf(org["numberOfEmployees"]); size != "" {
			add("size", size, "schema.org data")
		}
		add("about", firstText(org["description"]), "schema.org data")
	}

	meta := map[string]string{}
	for _, tag := range metaTag.FindAllString(string(page), -1) {
		var key, content string
		for _, m := range attr.FindAllStringSubmatch(tag, -1) {
			v := m[3] + m[4]
			switch strings.ToLower(m[1]) {
			case "name", "property":
				key = strings.ToLower(v)
			case "content":
				content = v
			}
		}
		if key != "" && meta[key] == "" {
			meta[key] = content
		}
	}
	add("name", meta["og:site_name"], "the page's title data")
	add("about", firstNonEmpty(meta["description"], meta["og:description"]), "the page's description")
	if m := telLink.FindStringSubmatch(string(page)); m != nil {
		v, _ := url.PathUnescape(m[1])
		add("phone", v, "a phone link")
	}

	text := pageText(page)
	if m := registration.FindStringSubmatch(text); m != nil {
		add("registrationNumber", cleanRegistration(m[1]), `the page's text ("`+quote(m[0])+`")`)
	} else if m := registrationAfterName.FindStringSubmatch(text); m != nil {
		add("registrationNumber", cleanRegistration(m[1]), `the page's text ("`+quote(m[0])+`")`)
	}
	return out
}

// ContactPage finds a link to the site's contact page (same site only).
func ContactPage(page []byte, pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	for _, m := range contactHref.FindAllStringSubmatch(string(page), -1) {
		u, err := base.Parse(html.UnescapeString(m[1]))
		if err == nil && u.Host == base.Host && (u.Scheme == "https" || u.Scheme == "http") && u.String() != pageURL {
			u.Fragment = ""
			return u.String()
		}
	}
	return ""
}

func suggestionValue(list []Suggestion, field string) string {
	for _, s := range list {
		if s.Field == field {
			return s.Value
		}
	}
	return ""
}

// Merge adds the suggestions of a later page for fields not found yet.
func Merge(found, more []Suggestion) []Suggestion {
	for _, s := range more {
		if suggestionValue(found, s.Field) == "" {
			found = append(found, s)
		}
	}
	return found
}

func pageText(page []byte) string {
	t := scripts.ReplaceAllString(string(page), " ")
	t = tags.ReplaceAllString(t, " ")
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(t), " "))
}

// quote is the matched text, short, as evidence.
func quote(match string) string {
	q := strings.TrimSpace(match)
	if len(q) > 80 {
		q = q[:80]
	}
	return q
}

func cleanRegistration(s string) string {
	s = strings.Trim(strings.ToUpper(spaces.ReplaceAllString(s, " ")), "() ")
	if strings.Count(s, "(") > strings.Count(s, ")") {
		s += ")"
	}
	return s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func first(v any) any {
	if arr, ok := v.([]any); ok {
		if len(arr) == 0 {
			return nil
		}
		return arr[0]
	}
	return v
}

func firstText(vs ...any) string {
	for _, v := range vs {
		switch t := first(v).(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return t
			}
		case map[string]any:
			if s := firstText(t["name"], t["@value"]); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		}
	}
	return ""
}

var orgTypes = []string{"Organization", "Corporation", "LocalBusiness", "ProfessionalService", "EmploymentAgency", "Store",
	"NGO", "EducationalOrganization", "GovernmentOrganization", "MedicalOrganization", "OnlineBusiness", "AccountingService",
	"FinancialService", "LegalService", "Hotel", "Restaurant"}

func isOrg(o map[string]any) bool {
	types := []string{}
	switch t := o["@type"].(type) {
	case string:
		types = append(types, t)
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok {
				types = append(types, s)
			}
		}
	}
	for _, t := range types {
		for _, want := range orgTypes {
			if strings.EqualFold(strings.TrimPrefix(t, "schema:"), want) {
				return true
			}
		}
	}
	return false
}

// organization finds the site's own Organization in its JSON-LD (preferring
// one whose url is on this site).
func organization(page []byte, pageURL string) map[string]any {
	host := ""
	if u, err := url.Parse(pageURL); err == nil {
		host = strings.TrimPrefix(u.Hostname(), "www.")
	}
	var found []map[string]any
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, x := range t {
				walk(x)
			}
		case map[string]any:
			if isOrg(t) {
				found = append(found, t)
			}
			for _, k := range []string{"@graph", "publisher", "author", "provider", "hiringOrganization"} {
				walk(t[k])
			}
		}
	}
	for _, m := range ldJSON.FindAllSubmatch(page, -1) {
		var data any
		if json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &data) == nil {
			walk(data)
		}
	}
	for _, o := range found {
		if u, err := url.Parse(firstText(o["url"])); err == nil && strings.TrimPrefix(u.Hostname(), "www.") == host {
			return o
		}
	}
	if len(found) > 0 {
		return found[0]
	}
	return nil
}

var countries = map[string]string{
	"malaysia": "MY", "singapore": "SG", "indonesia": "ID", "thailand": "TH", "philippines": "PH", "vietnam": "VN", "viet nam": "VN",
	"brunei": "BN", "brunei darussalam": "BN", "india": "IN", "china": "CN", "hong kong": "HK", "taiwan": "TW", "japan": "JP",
	"korea": "KR", "south korea": "KR", "australia": "AU", "new zealand": "NZ", "united kingdom": "GB", "uk": "GB", "great britain": "GB",
	"united states": "US", "united states of america": "US", "usa": "US", "united arab emirates": "AE", "uae": "AE", "germany": "DE",
	"france": "FR", "netherlands": "NL", "ireland": "IE", "canada": "CA", "saudi arabia": "SA", "qatar": "QA",
}

// CountryCode turns a country name or code into a two-letter code ("" when unknown).
func CountryCode(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 2 {
		return strings.ToUpper(s)
	}
	return countries[strings.ToLower(strings.TrimSuffix(s, "."))]
}

// SizeOf turns schema.org numberOfEmployees (a number, or a range) into a
// company size: micro 1–10, small 11–50, medium 51–200, large 201–1000,
// enterprise over 1000.
func SizeOf(v any) string {
	n := -1.0
	switch t := first(v).(type) {
	case float64:
		n = t
	case string:
		if f, err := strconv.ParseFloat(strings.ReplaceAll(t, ",", ""), 64); err == nil {
			n = f
		}
	case map[string]any:
		for _, k := range []string{"value", "maxValue", "minValue"} {
			if s := SizeOf(t[k]); s != "" {
				return s
			}
		}
	}
	switch {
	case n < 1:
		return ""
	case n <= 10:
		return "micro"
	case n <= 50:
		return "small"
	case n <= 200:
		return "medium"
	case n <= 1000:
		return "large"
	default:
		return "enterprise"
	}
}

// clip keeps the first n characters (the database's limits).
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
