package contacts

import "testing"

func TestLinkedinAddressesAreTheOnesTheCompanionOpens(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.linkedin.com/in/nur-aina-example":       "https://www.linkedin.com/in/nur-aina-example",
		"linkedin.com/in/nur-aina-example/":                  "https://www.linkedin.com/in/nur-aina-example",
		"https://my.linkedin.com/in/Nur%C3%A1?trk=x":         "https://www.linkedin.com/in/Nur%C3%A1",
		"https://lnkd.in/abc":                                "",
		"https://www.linkedin.com/pub/nur/1/2/3":             "",
		"https://www.linkedin.com/in/nur/details/experience": "",
		"https://evil.test/in/nur":                           "",
		"https://user:pw@www.linkedin.com/in/nur":            "",
	} {
		if got := ProfileURL(in); got != want {
			t.Errorf("ProfileURL(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"https://www.linkedin.com/company/maju-logistik":  "https://www.linkedin.com/company/maju-logistik/",
		"linkedin.com/company/maju-logistik/people/":      "https://www.linkedin.com/company/maju-logistik/",
		"https://www.linkedin.com/company/12345/":         "https://www.linkedin.com/company/12345/",
		"https://www.linkedin.com/school/maju":            "",
		"https://www.linkedin.com.evil.test/company/maju": "",
	} {
		if got := CompanyPageURL(in); got != want {
			t.Errorf("CompanyPageURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmploymentAnswersMustMakeSense(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		joined, left string
		current      *bool
		ok           bool
	}{
		{"2020-06", "", &yes, true},
		{"2020", "", &yes, true},
		{"2018-03", "2024-11", &no, true},
		{"2024", "2024", &no, true}, // the same year: in order at year precision
		{"", "", nil, true},         // unknown
		{"2020-06", "", nil, false},
		{"", "", &yes, false},
		{"2020-06", "2024-01", &yes, false},
		{"2020-06", "", &no, false},
		{"2024-05", "2024-02", &no, false},
		{"2020-13", "", &yes, false},
		{"1899", "", &yes, false},
		{"5 yrs", "", &yes, false},
	} {
		if got := ValidEmployment(c.joined, c.left, c.current); got != c.ok {
			t.Errorf("%q–%q current=%v: %v, want %v", c.joined, c.left, c.current, got, c.ok)
		}
	}
}

func TestWhereALeaverMovedIsTheirNewCompanysExactPage(t *testing.T) {
	ok := LinkedinMove{CompanyURL: "https://www.linkedin.com/company/acme-bhd/", CompanyName: "  Acme   Bhd ", Title: "HR Manager", Since: "2025-09"}
	got, valid := CleanMoves([]LinkedinMove{ok, ok})
	if !valid || len(got) != 1 || got[0].CompanyName != "Acme Bhd" {
		t.Fatalf("got %+v %v: tidied, each company once", got, valid)
	}
	for _, bad := range []LinkedinMove{
		{CompanyURL: "https://www.linkedin.com/company/acme-bhd", CompanyName: "Acme"}, // not the exact page form
		{CompanyURL: "https://evil.test/company/acme/", CompanyName: "Acme"},
		{CompanyURL: "https://www.linkedin.com/company/acme/", CompanyName: " "},
		{CompanyURL: "https://www.linkedin.com/company/acme/", CompanyName: "Acme", Since: "Sep 2025"},
	} {
		if _, valid := CleanMoves([]LinkedinMove{bad}); valid {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, valid := CleanMoves(make([]LinkedinMove, 4)); valid {
		t.Error("more than 3 accepted")
	}
}
