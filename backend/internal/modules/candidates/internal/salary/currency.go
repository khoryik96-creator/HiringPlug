package salary

import (
	"math"
	"regexp"
	"strings"
)

// Currency considers only the salary wording, never nationality or location.
func Currency(raw string) string { return currencyHint(raw, false) }

func currencyHint(raw string, allowWithoutAmount bool) string {
	t := strings.ToUpper(strings.ReplaceAll(raw, ",", ""))
	first := digitRE.FindStringIndex(t)
	if first == nil {
		if !allowWithoutAmount {
			return ""
		}
		first = []int{0}
	}
	best, code := math.MaxInt, ""
	for _, a := range currencyAliases {
		currency, alias := a.currency, a.alias
		for _, match := range a.re.FindAllStringIndex(t, -1) {
			start := match[0] + strings.Index(t[match[0]:match[1]], alias)
			// Glued ISO codes need an amount-shaped number (PHP7.4 is software).
			if len(alias) == 3 && alias == currency {
				at := strings.Index(t[start:match[1]], alias) + start
				tail := t[at+len(alias):]
				if len(tail) > 0 && tail[0] >= '0' && tail[0] <= '9' && !gluedAmountRE.MatchString(tail) {
					continue
				}
			}
			distance := start - first[0]
			if distance < 0 {
				distance = first[0] - (start + len(alias))
				if distance < 0 {
					distance = 0
				}
			}
			if distance < best || distance == best && currency < code {
				best, code = distance, currency
			}
		}
	}
	if code == "" && strings.Contains(raw, "$") {
		return "USD"
	}
	return code
}

var (
	digitRE       = regexp.MustCompile(`\d`)
	gluedAmountRE = regexp.MustCompile(`^(\d{3,}(?:\.\d+)?|\d+(?:\.\d+)?[KML])\b|^\d+(?:\.\d+)?%`)
)

type currencyAlias struct {
	currency, alias string
	re              *regexp.Regexp
}

// currencyAliases are compiled once: Currency runs for every salary component.
var currencyAliases = func() []currencyAlias {
	var out []currencyAlias
	for currency, aliases := range map[string][]string{
		"MYR": {"MYR", "RM", "RINGGIT"}, "SGD": {"SGD", "S$", "SINGAPORE DOLLAR"},
		"USD": {"USD", "US$", "UNITED STATES DOLLAR"}, "HKD": {"HKD", "HK$", "HONG KONG DOLLAR"},
		"AUD": {"AUD", "A$", "AUSTRALIAN DOLLAR"}, "GBP": {"GBP", "£", "POUND", "STERLING"}, "EUR": {"EUR", "€", "EURO"},
		"INR": {"INR", "₹", "RS.", "RUPEE", "LAKH", "LAKHS", "LAC", "LACS", "LPA", "L", "CR", "CRORE", "CRORES"},
		"IDR": {"IDR", "RP", "RUPIAH"}, "BND": {"BND", "BRUNEI DOLLAR"},
		"THB": {"THB", "BAHT"}, "KHR": {"KHR", "RIEL"}, "VND": {"VND", "DONG", "ĐỒNG"},
		"CNY": {"CNY", "RMB", "YUAN"}, "LAK": {"LAK", "LAO KIP"}, "MMK": {"MMK", "KYAT"}, "PHP": {"PHP", "PESO"},
	} {
		for _, alias := range aliases {
			out = append(out, currencyAlias{currency, alias, regexp.MustCompile(`(^|[^A-Z]|[0-9][KML])` + regexp.QuoteMeta(alias) + `([^A-Z]|$)`)})
		}
	}
	return out
}()
