// Package country names countries by their ISO 3166-1 alpha-2 code, for
// documents the server writes (the web app uses the browser's own names).
package country

var names = map[string]string{
	"MY": "Malaysia", "SG": "Singapore", "ID": "Indonesia", "TH": "Thailand", "PH": "Philippines", "VN": "Vietnam",
	"BN": "Brunei", "KH": "Cambodia", "MM": "Myanmar", "LA": "Laos", "IN": "India", "CN": "China", "HK": "Hong Kong",
	"TW": "Taiwan", "JP": "Japan", "KR": "South Korea", "AU": "Australia", "NZ": "New Zealand", "GB": "United Kingdom",
	"US": "United States", "CA": "Canada", "AE": "United Arab Emirates", "SA": "Saudi Arabia", "QA": "Qatar",
	"DE": "Germany", "FR": "France", "NL": "Netherlands", "IE": "Ireland", "CH": "Switzerland", "BD": "Bangladesh",
	"PK": "Pakistan", "LK": "Sri Lanka", "NP": "Nepal",
}

// Name returns a country's English name, or the code itself when unknown.
func Name(code string) string {
	if n, ok := names[code]; ok {
		return n
	}
	return code
}
