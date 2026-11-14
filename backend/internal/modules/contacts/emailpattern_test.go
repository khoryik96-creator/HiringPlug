package contacts

import "testing"

func TestGuessingAnAddressFromColleagues(t *testing.T) {
	maju := []Person{
		{"Siti", "Aisyah", "siti.aisyah@maju.com.my"},
		{"Wei", "Chen", "wei.chen@maju.com.my"},
		{"José", "Núñez", "jose.nunez@maju.com.my"},
		{"Hr", "", "hr@maju.com.my"},
		{"Old", "Domain", "old.domain@maju-old.test"},
	}
	guess, ok := GuessEmail(Person{FirstName: "Ana", LastName: "Lim"}, maju)
	if !ok || guess != (EmailGuess{Email: "ana.lim@maju.com.my", Pattern: "first.last", Matches: 3, Of: 4}) {
		t.Fatalf("got %+v %v", guess, ok)
	}
	if g, _ := GuessEmail(Person{FirstName: "Nur Aina", LastName: "binti Ahmad"}, maju); g.Email != "nuraina.bintiahmad@maju.com.my" {
		t.Errorf("several words: %q", g.Email)
	}

	initials := []Person{{"Siti", "Aisyah", "saisyah@acme.test"}, {"Wei", "Chen", "wchen@acme.test"}}
	if g, ok := GuessEmail(Person{FirstName: "Ana", LastName: "Lim"}, initials); !ok || g.Email != "alim@acme.test" || g.Pattern != "flast" {
		t.Errorf("flast: %+v %v", g, ok)
	}
	// First names only: no last name is needed.
	firsts := []Person{{"Siti", "Aisyah", "siti@acme.test"}, {"Wei", "Chen", "wei@acme.test"}}
	if g, ok := GuessEmail(Person{FirstName: "Ana"}, firsts); !ok || g.Email != "ana@acme.test" {
		t.Errorf("first: %+v %v", g, ok)
	}

	for name, colleagues := range map[string][]Person{
		"one colleague is not a pattern": {{"Siti", "Aisyah", "siti.aisyah@maju.com.my"}},
		"two patterns as likely":         {{"Siti", "Aisyah", "siti.aisyah@x.test"}, {"Wei", "Chen", "wchen@x.test"}},
		"addresses that follow no name":  {{"Siti", "Aisyah", "hr@x.test"}, {"Wei", "Chen", "sales@x.test"}},
	} {
		if g, ok := GuessEmail(Person{FirstName: "Ana", LastName: "Lim"}, colleagues); ok {
			t.Errorf("%s: guessed %+v", name, g)
		}
	}
	if _, ok := GuessEmail(Person{FirstName: "Ana"}, maju); ok {
		t.Error("first.last needs a last name")
	}
}
