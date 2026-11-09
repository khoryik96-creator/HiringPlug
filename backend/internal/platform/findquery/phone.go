package findquery

import (
	"slices"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// Preferred are the countries a number typed without its country code is
// read as first (after the agency's own): where most candidates are from
// (Malaysia, Singapore, Indonesia, India, Pakistan, Bangladesh), then the
// UK, the US and the larger European countries. A number possible there is
// enough; elsewhere it must be a valid number of that country.
var Preferred = []string{"MY", "SG", "ID", "IN", "PK", "BD", "GB", "US", "DE", "FR", "IT", "ES", "NL", "IE", "CH", "AU"}

var allRegions = func() []string {
	out := make([]string, 0, 250)
	for r := range phonenumbers.GetSupportedRegions() {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}()

// cleanPhone drops the formatting from a phone number; a leading 00 is +.
func cleanPhone(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(" ().-\t", r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if rest, ok := strings.CutPrefix(s, "00"); ok {
		return "+" + rest
	}
	return s
}

// Numbers returns the whole numbers (E.164, how candidates' phones are
// stored) that the phone number typed can be, the likeliest first. With a
// country code (+ or 00) it is that one number. Without, it is the number
// read as already including its country code, then as a local number of
// home, of the Preferred countries and of every other country where it is
// valid. Only a whole number matches: 12 345 6789 does not find
// +60 11 2345 6789.
func Numbers(typed, home string) []string {
	clean := cleanPhone(typed)
	if !completePhone.MatchString(clean) {
		return nil
	}
	var out []string
	add := func(n *phonenumbers.PhoneNumber) {
		if e := phonenumbers.Format(n, phonenumbers.E164); !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	if !strings.HasPrefix(clean, "0") {
		if n, err := phonenumbers.Parse("+"+strings.TrimPrefix(clean, "+"), "ZZ"); err == nil && phonenumbers.IsPossibleNumber(n) {
			add(n)
		}
	}
	if strings.HasPrefix(clean, "+") {
		return out
	}
	preferred := append([]string{strings.ToUpper(home)}, Preferred...)
	for _, region := range append(slices.Clone(preferred), allRegions...) {
		n, err := phonenumbers.Parse(clean, region)
		if err != nil || !phonenumbers.IsPossibleNumber(n) {
			continue
		}
		if slices.Contains(preferred, region) || phonenumbers.IsValidNumberForRegion(n, region) {
			add(n)
		}
	}
	return out
}

// HasCountryCode reports a phone number typed with its country code.
func HasCountryCode(typed string) bool { return strings.HasPrefix(cleanPhone(typed), "+") }

// CountryCode is the calling code of a stored (E.164) number, e.g. 60.
func CountryCode(e164 string) int32 {
	n, err := phonenumbers.Parse(e164, "ZZ")
	if err != nil {
		return 0
	}
	return n.GetCountryCode()
}
