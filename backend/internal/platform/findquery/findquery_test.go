package findquery

import (
	"slices"
	"testing"

	"github.com/nyaruka/phonenumbers"
)

// The cases come from Kano's own tests of /find (find_core.js and its
// suites), so the search box reads what is typed the way Kano does.

func TestSplitOnCommasFoldsRepeatsAndStopsAtSix(t *testing.T) {
	got, skipped := Split("alia  abu, Sam, sam ,, x,y,z,w,v,u")
	if want := []string{"alia abu", "Sam", "x", "y", "z", "w"}; !slices.Equal(got, want) {
		t.Fatalf("Split = %q, want %q", got, want)
	}
	if skipped != 2 {
		t.Fatalf("skipped = %d, want 2 (v and u)", skipped)
	}
}

func TestParseTellsNamesEmailsPhonesAndCompanies(t *testing.T) {
	for _, c := range []struct {
		typed          string
		kind           Kind
		value, company string
	}{
		{"Ali Abu - Nintex", Name, "Ali Abu", "Nintex"},
		{"Anne-Marie Lee", Name, "Anne-Marie Lee", ""},
		{"Ali - K-Corp - Malaysia", Name, "Ali", "K-Corp - Malaysia"},
		{"aaa@gmail.com", Email, "aaa@gmail.com", ""},
		{`aaa\@gmail.com`, Email, "aaa@gmail.com", ""},
		{"aaa@gmail.com - Maybank", Email, "aaa@gmail.com", "Maybank"},
		{"+60 (12) 941-211", Phone, "+60 (12) 941-211", ""},
		{"+60 12 - 941211", Phone, "+60 12 - 941211", ""},
		{"012 - 941211", Phone, "012 - 941211", ""},
		{"+1 202 - 555 - 0123", Phone, "+1 202 - 555 - 0123", ""},
		{"312345", Phone, "312345", ""},
		{"7290", Phone, "7290", ""},
		{"+60 12 - 941211 - Nintex Malaysia", Phone, "+60 12 - 941211", "Nintex Malaysia"},
	} {
		s := Parse(c.typed)
		if s.Error != "" || s.Kind != c.kind || s.Value != c.value || s.Company != c.company {
			t.Errorf("Parse(%q) = %+v, want %s %q company %q", c.typed, s, c.kind, c.value, c.company)
		}
	}
}

func TestParseRefusesWhatIsIncomplete(t *testing.T) {
	for typed, want := range map[string]string{
		"Ali -":             ErrCompany,
		"- Nintex":          ErrCompany,
		"Ali - !!!":         ErrCompany,
		"a@@gmail.com":      ErrEmail,
		"aisyah@":           ErrEmail,
		"123":               ErrPhone,
		"+1234567890123456": ErrPhone,
		"12+34567":          ErrPhone,
	} {
		if got := Parse(typed).Error; got != want {
			t.Errorf("Parse(%q).Error = %q, want %q", typed, got, want)
		}
	}
}

func TestPhoneNumbersMatchTheWholeNumber(t *testing.T) {
	// Each pair is the same number written two ways; the first is how it
	// is stored (E.164). Either way of writing it finds the stored one.
	for _, c := range [][2]string{
		{"+6012941211", "012941211"}, {"+6012941211", "6012941211"}, {"+6012941211", "(012) 941211"},
		{"+6012941211", "+60 12-941211"}, {"+6012941211", "006012941211"},
		{"+447911123456", "07911 123456"}, {"+61294121111", "02 9412 1111"},
		{"+6581234567", "81234567"}, {"+6581234567", "8123 4567"}, {"+12025550123", "(202) 555-0123"},
		{"+390612345678", "06 1234 5678"}, {"+79123456789", "8 912 345 6789"},
		{"+376312345", "312345"}, {"+68271234", "71234"}, {"+6907290", "7290"},
		{"+628123456789", "0812-3456-789"}, {"+919876543210", "098765 43210"}, {"+923001234567", "0300 1234567"},
		{"+4915123456789", "01512 3456789"}, {"+33612345678", "06 12 34 56 78"},
	} {
		stored, typed := c[0], c[1]
		if !slices.Contains(Numbers(typed, "MY"), stored) {
			t.Errorf("%q does not find %s; it reads as %v", typed, stored, Numbers(typed, "MY"))
		}
		if got := Numbers(stored, "MY"); !slices.Equal(got, []string{stored}) {
			t.Errorf("%s with its country code is %v, want only itself", stored, got)
		}
	}
	for _, c := range [][2]string{
		{"+6012941211", "+6512941211"}, {"+6012941211", "011941211"}, {"+12025550123", "+1 303 555 0123"},
		{"+6012941211", "+99912941211"}, {"+376312345", "+299312345"}, {"+376312345", "312346"},
		{"+6907290", "7291"}, {"+60123456789", "23456789"}, {"+60123456789", "3456789"},
	} {
		if slices.Contains(Numbers(c[1], "MY"), c[0]) {
			t.Errorf("%q must not find %s (only the whole number matches)", c[1], c[0])
		}
	}
	if Numbers("123", "MY") != nil {
		t.Error("three digits are no phone number")
	}
}

func TestEveryCountrysMobileIsFoundHoweverItIsWritten(t *testing.T) {
	for region := range phonenumbers.GetSupportedRegions() {
		n := phonenumbers.GetExampleNumberForType(region, phonenumbers.MOBILE)
		if n == nil {
			continue
		}
		stored := phonenumbers.Format(n, phonenumbers.E164)
		forms := []string{stored, stored[1:], phonenumbers.Format(n, phonenumbers.INTERNATIONAL), phonenumbers.GetNationalSignificantNumber(n)}
		if region != "AR" { // the library writes Argentine mobiles "09 15-...", which nobody types (see below)
			forms = append(forms, phonenumbers.Format(n, phonenumbers.NATIONAL))
		}
		for _, typed := range forms {
			if !slices.Contains(Numbers(typed, "MY"), stored) {
				t.Errorf("%s: %q does not find %s", region, typed, stored)
			}
		}
	}
}

func TestArgentineMobilesAsTheyAreWritten(t *testing.T) {
	if got := Numbers("011 15-2345-6789", "AR"); !slices.Contains(got, "+5491123456789") {
		t.Fatalf("Numbers = %v", got)
	}
}

func TestLocalNumbersReadAsTheAgencysCountryFirst(t *testing.T) {
	if got := Numbers("012 345 6789", "MY"); len(got) == 0 || got[0] != "+60123456789" {
		t.Fatalf("Numbers = %v, want Malaysia first", got)
	}
	if got := Numbers("8123 4567", "SG"); got[0] != "+6581234567" {
		t.Fatalf("Numbers = %v, want Singapore first", got)
	}
	if !HasCountryCode("0060 12 345 6789") || HasCountryCode("012 345 6789") {
		t.Fatal("HasCountryCode")
	}
	if CountryCode("+6581234567") != 65 {
		t.Fatal("CountryCode")
	}
}

func TestNameScore(t *testing.T) {
	for _, c := range []struct {
		query, name string
		ok          func(float64) bool
	}{
		{"Amirul Hakim", "Hakim Amirul", func(s float64) bool { return s > 0.95 && s < 1 }},
		{"Amirul Hakim", "Amirul bin Hakim", func(s float64) bool { return s == 1 }},
		{"Siti Aisyah", "Siti binti Aisyah", func(s float64) bool { return s == 1 }},
		{"Mohamad Ali", "Muhamad Ali", func(s float64) bool { return s >= MinNameScore }},
		{"amir", "Amirul", func(s float64) bool { return s >= MinNameScore }},
		{"José", "Jose", func(s float64) bool { return s == 1 }},
		{"Tuck Wai Foong", "Foong Tuck Wai", func(s float64) bool { return s > 0.95 }},
		{"Sam Liew", "Peter Parker", func(s float64) bool { return s < MinNameScore }},
		{"陈伟", "陈伟", func(s float64) bool { return s == 1 }},
	} {
		if s := NameScore(c.query, c.name); !c.ok(s) {
			t.Errorf("NameScore(%q, %q) = %.3f", c.query, c.name, s)
		}
	}
	if got := NameProbes("Mohamad bin Ali"); !slices.Equal(got, []string{"mohamad", "ali", "moha"}) {
		t.Errorf("NameProbes = %q", got)
	}
}

func TestWorksAtMatchesWholeWords(t *testing.T) {
	for employer, want := range map[string]bool{
		"Nintex Malaysia Sdn Bhd": true, "NINTEX": true, "Nintexify": false, "": false, "Néntex": false,
	} {
		if WorksAt(employer, "nintex") != want {
			t.Errorf("WorksAt(%q, nintex) != %v", employer, want)
		}
	}
	if !WorksAt("Société Générale", "societe generale") || !WorksAt("K-Corp (M) Bhd", "k corp") {
		t.Error("accents and punctuation are folded")
	}
	if WorksAt("Nintex", "!!!") {
		t.Error("a company with no words matches nothing")
	}
}
