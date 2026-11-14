package contacts

// Email patterns: how a company spells its people's addresses
// (first.last@, flast@, first@, ...), learned from colleagues whose mailbox
// on record, to suggest an address for a contact without one (then verified
// like any address).

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Person is a name and, if known, an email address.
type Person struct{ FirstName, LastName, Email string }

// EmailGuess is a suggested address, and how sure it is.
type EmailGuess struct {
	Email   string
	Pattern string // e.g. "first.last"
	Matches int    // colleagues whose address follows it
	Of      int    // colleagues with an address at this domain
}

var patterns = []struct {
	name string
	make func(first, last string) string
}{
	{"first.last", func(f, l string) string { return f + "." + l }},
	{"firstlast", func(f, l string) string { return f + l }},
	{"f.last", func(f, l string) string { return initial(f) + "." + l }},
	{"flast", func(f, l string) string { return initial(f) + l }},
	{"first_last", func(f, l string) string { return f + "_" + l }},
	{"first-last", func(f, l string) string { return f + "-" + l }},
	{"last.first", func(f, l string) string { return l + "." + f }},
	{"lastfirst", func(f, l string) string { return l + f }},
	{"first.l", func(f, l string) string { return f + "." + initial(l) }},
	{"firstl", func(f, l string) string { return f + initial(l) }},
	{"first", func(f, _ string) string { return f }},
	{"last", func(_, l string) string { return l }},
}

func initial(s string) string {
	if s == "" {
		return ""
	}
	return s[:1]
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// nameKeys returns the forms a name part takes in addresses: all its words
// joined ("nuraina"), and its first word ("nur"), in plain ASCII letters.
func nameKeys(name string) []string {
	plain, _, _ := transform.String(stripMarks, strings.ToLower(name))
	var words []string
	for _, w := range strings.Fields(plain) {
		w = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r
			}
			return -1
		}, w)
		if w != "" {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return nil
	}
	keys := []string{strings.Join(words, "")}
	if len(words) > 1 {
		keys = append(keys, words[0])
	}
	return keys
}

func split(email string) (local, domain string, ok bool) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", "", false
	}
	return strings.ToLower(email[:at]), strings.ToLower(email[at+1:]), true
}

// GuessEmail suggests an address for person from colleagues' addresses
// (verified ones), when at least two at the company's main domain follow the
// same pattern and no other pattern is followed as often. A pattern needing
// a last name is not used without one.
func GuessEmail(person Person, colleagues []Person) (EmailGuess, bool) {
	domains := map[string]int{}
	for _, c := range colleagues {
		if _, d, ok := split(c.Email); ok {
			domains[d]++
		}
	}
	domain, most := "", 0
	for d, n := range domains {
		if n > most || (n == most && d < domain) {
			domain, most = d, n
		}
	}
	if most < 2 {
		return EmailGuess{}, false
	}
	type vote struct {
		name  string
		first int // which first-name form (0: all words, 1: first word)
	}
	votes := map[vote]int{}
	for _, c := range colleagues {
		local, d, ok := split(c.Email)
		if !ok || d != domain {
			continue
		}
		firsts, lasts := nameKeys(c.FirstName), nameKeys(c.LastName)
		seen := map[vote]bool{}
		for i, f := range firsts {
			for _, p := range patterns {
				needsLast := p.name != "first"
				for _, l := range append(lasts, "") {
					if (needsLast && l == "") || p.make(f, l) != local {
						continue
					}
					if v := (vote{p.name, i}); !seen[v] {
						seen[v] = true
						votes[v]++
					}
				}
			}
		}
	}
	ranked := make([]vote, 0, len(votes))
	for v := range votes {
		ranked = append(ranked, v)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if votes[ranked[i]] != votes[ranked[j]] {
			return votes[ranked[i]] > votes[ranked[j]]
		}
		if order(ranked[i].name) != order(ranked[j].name) {
			return order(ranked[i].name) < order(ranked[j].name)
		}
		return ranked[i].first < ranked[j].first
	})
	if len(ranked) == 0 || votes[ranked[0]] < 2 || (len(ranked) > 1 && votes[ranked[1]] == votes[ranked[0]] && ranked[1].name != ranked[0].name) {
		return EmailGuess{}, false // too few, or two patterns as likely
	}
	best := ranked[0]
	firsts, lasts := nameKeys(person.FirstName), nameKeys(person.LastName)
	if len(firsts) == 0 {
		return EmailGuess{}, false
	}
	first := firsts[min(best.first, len(firsts)-1)]
	last := ""
	if len(lasts) > 0 {
		last = lasts[0]
	}
	for _, p := range patterns {
		if p.name != best.name {
			continue
		}
		if p.name != "first" && last == "" {
			return EmailGuess{}, false
		}
		return EmailGuess{Email: p.make(first, last) + "@" + domain, Pattern: p.name, Matches: votes[best], Of: most}, true
	}
	return EmailGuess{}, false
}

func order(name string) int {
	for i, p := range patterns {
		if p.name == name {
			return i
		}
	}
	return len(patterns)
}

// SuggestEmail suggests an address for a contact from their colleagues'.
func (s *Service) SuggestEmail(ctx context.Context, p authz.Principal, contactID string) (*EmailGuess, error) {
	var out *EmailGuess
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, contactID)
		if err != nil {
			return err
		}
		rows, err := q.ColleagueAddresses(ctx, store.ColleagueAddressesParams{CompanyID: row.CompanyID, ContactID: contactID})
		if err != nil {
			return err
		}
		colleagues := make([]Person, len(rows))
		for i, r := range rows {
			colleagues[i] = Person{FirstName: r.FirstName, LastName: r.LastName, Email: r.Email}
		}
		if g, ok := GuessEmail(Person{FirstName: row.FirstName, LastName: row.LastName}, colleagues); ok {
			out = &g
		}
		return nil
	})
	return out, err
}

// SuggestContactEmail handles GET /v1/contacts/{contactId}/email-suggestion.
func (h *HTTP) SuggestContactEmail(ctx context.Context, req api.SuggestContactEmailRequestObject) (api.SuggestContactEmailResponseObject, error) {
	g, err := h.svc.SuggestEmail(ctx, authz.MustFromContext(ctx), req.ContactId)
	if err != nil {
		return nil, err
	}
	out := api.SuggestContactEmail200JSONResponse{}
	out.Suggestion.SetNull()
	if g != nil {
		out.Suggestion.Set(api.EmailGuess{Email: g.Email, Pattern: g.Pattern, Matches: g.Matches, Of: g.Of})
	}
	return out, nil
}
