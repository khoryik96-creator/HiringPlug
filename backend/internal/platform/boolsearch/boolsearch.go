// Package boolsearch reads a recruiter's boolean search, the way job boards
// take it: SAP AND (BW OR BPC) NOT internship, "project manager", java*.
//
//   - words next to each other must all appear (AND is implied);
//   - OR between alternatives; NOT (or -word) leaves out;
//   - ( ) groups; "quoted words" must appear together, in that order;
//   - word* is any word starting so (manag* finds manager, managing);
//   - NEAR (or NEAR/n, W/n) between two words or "phrases": within n words
//     of each other, either way round (NEAR alone: within 5).
//
// NEAR binds tightest, then NOT, then AND, then OR. The operators are read in any case
// (and, or, not). The query becomes a PostgreSQL text search query
// (to_tsquery('english', ...)), so words match their other forms
// (manager, managers) as CV text is indexed.
package boolsearch

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Limits on a query (the contract allows 1000 characters).
const (
	MaxTerms = 50
	MaxDepth = 10
	// MaxNear is the widest NEAR/n; NEAR alone is DefaultNear.
	MaxNear     = 20
	DefaultNear = 5
)

// Query is a parsed boolean search.
type Query struct {
	root node
	// Terms are the words and phrases looked for (not those left out with
	// NOT), in the order written, without repeats: to highlight in a CV.
	Terms []Term
}

// Term is a word or phrase to highlight; Prefix for word*.
type Term struct {
	Words  []string
	Prefix bool
}

// Text is the term as written ("project manager", "manag*").
func (t Term) Text() string {
	s := strings.Join(t.Words, " ")
	if t.Prefix {
		s += "*"
	}
	return s
}

// Parse reads a boolean search. Its errors are written for the person who
// typed the query.
func Parse(input string) (Query, error) {
	toks, err := lex(input)
	if err != nil {
		return Query{}, err
	}
	if len(toks) == 0 {
		return Query{}, errors.New("is empty: type words to look for")
	}
	p := &parser{toks: toks}
	root, err := p.or(0)
	if err != nil {
		return Query{}, err
	}
	if p.pos < len(p.toks) {
		if p.toks[p.pos].kind == tokClose {
			return Query{}, errors.New("has a ) without its (")
		}
		return Query{}, fmt.Errorf("could not be read near %q", p.toks[p.pos].text)
	}
	if p.terms > MaxTerms {
		return Query{}, fmt.Errorf("has more than %d words: shorten it", MaxTerms)
	}
	q := Query{root: root}
	seen := map[string]bool{}
	root.collect(false, func(t Term) {
		key := strings.ToLower(t.Text())
		if !seen[key] {
			seen[key] = true
			q.Terms = append(q.Terms, t)
		}
	})
	return q, nil
}

// ParseField reads the boolean search sent in an API field: what is wrong
// with a mistyped one is that field's validation error.
func ParseField(field, input string) (Query, error) {
	q, err := Parse(input)
	if err != nil {
		return Query{}, apperr.Validation(apperr.FieldErrors{{Field: field, Message: err.Error()}})
	}
	return q, nil
}

// TSQueryOf is the to_tsquery text of an optional boolean search sent in a
// field (nil when none is).
func TSQueryOf(field string, input *string) (*string, error) {
	if input == nil {
		return nil, nil //nolint:nilnil // nil: no boolean search was asked for
	}
	q, err := ParseField(field, *input)
	if err != nil {
		return nil, err
	}
	ts := q.TSQuery()
	return &ts, nil
}

// TSQuery is the query for to_tsquery('english', ...).
func (q Query) TSQuery() string { return q.root.ts() }

// Excluded is a query matching any word or phrase left out with NOT ("" when
// none is): a candidate whose CV has one is left out even when their profile
// matches.
func (q Query) Excluded() string {
	var parts []string
	var walk func(n node, negated bool)
	walk = func(n node, negated bool) {
		switch v := n.(type) {
		case termNode:
			if negated {
				parts = append(parts, v.ts())
			}
		case andNode:
			walk(v.l, negated)
			walk(v.r, negated)
		case orNode:
			walk(v.l, negated)
			walk(v.r, negated)
		case notNode:
			walk(v.n, !negated)
		case nearNode:
			if negated {
				parts = append(parts, v.ts())
			}
		}
	}
	walk(q.root, false)
	return strings.Join(parts, " | ")
}

// --- reading ---

type tokKind int

const (
	tokWord tokKind = iota
	tokPhrase
	tokOpen
	tokClose
	tokAnd
	tokOr
	tokNot
	tokNear
)

type token struct {
	kind   tokKind
	text   string
	prefix bool
	near   int // NEAR's distance
}

// nearWord reads NEAR, NEAR/n or W/n: the distance, or 0 when it is not one.
func nearWord(raw string) (int, error) {
	up := strings.ToUpper(raw)
	if up == "NEAR" {
		return DefaultNear, nil
	}
	var n string
	switch {
	case strings.HasPrefix(up, "NEAR/"):
		n = up[len("NEAR/"):]
	case strings.HasPrefix(up, "W/"):
		n = up[len("W/"):]
	default:
		return 0, nil
	}
	d := 0
	for _, r := range n {
		if r < '0' || r > '9' {
			return 0, nil // not NEAR/n: an ordinary word
		}
		d = d*10 + int(r-'0')
		if d > MaxNear {
			return 0, fmt.Errorf("has %s: NEAR goes up to %d words", raw, MaxNear)
		}
	}
	if n == "" && strings.HasPrefix(up, "W/") {
		return 0, nil // "manager w/ SAP": w/ for "with", an ordinary word
	}
	if n == "" || d == 0 {
		return 0, fmt.Errorf("has %s: write NEAR/1 to NEAR/%d", raw, MaxNear)
	}
	return d, nil
}

func lex(s string) ([]token, error) {
	var out []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r) || r == ',':
			i++
		case r == '(':
			out = append(out, token{kind: tokOpen, text: "("})
			i++
		case r == ')':
			out = append(out, token{kind: tokClose, text: ")"})
			i++
		case r == '"' || r == '“' || r == '”':
			j := i + 1
			for j < len(rs) && rs[j] != '"' && rs[j] != '“' && rs[j] != '”' {
				j++
			}
			if j == len(rs) {
				return nil, errors.New(`has a " without its closing "`)
			}
			if words := wordsOf(string(rs[i+1 : j])); len(words) > 0 {
				out = append(out, token{kind: tokPhrase, text: strings.Join(words, " ")})
			}
			i = j + 1
		case (r == '-' || r == '!') && (i == 0 || unicode.IsSpace(rs[i-1]) || rs[i-1] == '(') && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]):
			out = append(out, token{kind: tokNot, text: string(r)}) // -internship, !internship
			i++
		case r == '+' && (i == 0 || unicode.IsSpace(rs[i-1]) || rs[i-1] == '('):
			i++ // +word: the word must appear, as any word does
		default:
			j := i
			for j < len(rs) && !unicode.IsSpace(rs[j]) && !strings.ContainsRune(`()",“”`, rs[j]) {
				j++
			}
			raw := string(rs[i:j])
			i = j
			switch strings.ToUpper(raw) {
			case "AND", "&&", "&":
				out = append(out, token{kind: tokAnd, text: raw})
				continue
			case "OR", "||", "|":
				out = append(out, token{kind: tokOr, text: raw})
				continue
			case "NOT", "!":
				out = append(out, token{kind: tokNot, text: raw})
				continue
			}
			if d, err := nearWord(raw); err != nil {
				return nil, err
			} else if d > 0 {
				out = append(out, token{kind: tokNear, text: raw, near: d})
				continue
			}
			prefix := strings.HasSuffix(raw, "*")
			words := wordsOf(strings.TrimRight(raw, "*"))
			switch {
			case len(words) == 1:
				out = append(out, token{kind: tokWord, text: words[0], prefix: prefix})
			case len(words) > 1: // node.js, e-commerce: its parts together
				out = append(out, token{kind: tokPhrase, text: strings.Join(words, " ")})
			}
		}
	}
	return out, nil
}

// wordsOf splits text into the words a text search indexes: letters and
// digits (C++ is C, .NET is NET).
func wordsOf(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) })
}

type parser struct {
	toks  []token
	pos   int
	terms int
}

func (p *parser) peek() (token, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return token{}, false
}

func (p *parser) or(depth int) (node, error) {
	left, err := p.and(depth)
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokOr {
			return left, nil
		}
		p.pos++
		right, err := p.and(depth)
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
}

func (p *parser) and(depth int) (node, error) {
	left, err := p.near(depth)
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind == tokOr || t.kind == tokClose {
			return left, nil
		}
		if t.kind == tokAnd {
			p.pos++
		}
		right, err := p.near(depth)
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
}

// near reads a NEAR b: two words or phrases within a few words of each other.
func (p *parser) near(depth int) (node, error) {
	left, err := p.unary(depth)
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tokNear {
			return left, nil
		}
		p.pos++
		right, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		l, lok := left.(termNode)
		r, rok := right.(termNode)
		if !lok || !rok {
			return nil, fmt.Errorf("has %s between something other than two words or \"phrases\"", strings.ToUpper(t.text))
		}
		left = nearNode{l, r, t.near}
	}
}

func (p *parser) unary(depth int) (node, error) {
	t, ok := p.peek()
	if !ok {
		return nil, errors.New("ends with AND, OR or NOT: add what to look for after it")
	}
	switch t.kind {
	case tokNot:
		p.pos++
		inner, err := p.near(depth) // NOT a NEAR b leaves out the pair
		if err != nil {
			return nil, err
		}
		return notNode{inner}, nil
	case tokOpen:
		if depth >= MaxDepth {
			return nil, fmt.Errorf("has brackets more than %d deep", MaxDepth)
		}
		p.pos++
		inner, err := p.or(depth + 1)
		if err != nil {
			return nil, err
		}
		if c, ok := p.peek(); !ok || c.kind != tokClose {
			return nil, errors.New("has a ( without its )")
		}
		p.pos++
		return inner, nil
	case tokWord:
		p.pos++
		p.terms++
		return termNode{Term{Words: []string{t.text}, Prefix: t.prefix}}, nil
	case tokPhrase:
		p.pos++
		p.terms++
		return termNode{Term{Words: strings.Fields(t.text)}}, nil
	case tokClose:
		return nil, errors.New("has a ) without its (, or empty brackets")
	case tokNear:
		return nil, fmt.Errorf("has %s with nothing before it", strings.ToUpper(t.text))
	default: // AND / OR with nothing before it
		return nil, fmt.Errorf("has %s with nothing before it", strings.ToUpper(t.text))
	}
}

// --- the tree ---

type node interface {
	ts() string
	collect(negated bool, add func(Term))
}

type termNode struct{ t Term }
type andNode struct{ l, r node }
type orNode struct{ l, r node }
type notNode struct{ n node }
type nearNode struct {
	l, r termNode
	d    int
}

func lexeme(w string) string { return "'" + w + "'" } // words hold only letters, digits and marks

func (n termNode) ts() string {
	if len(n.t.Words) == 1 {
		if n.t.Prefix {
			return lexeme(n.t.Words[0]) + ":*"
		}
		return lexeme(n.t.Words[0])
	}
	parts := make([]string, len(n.t.Words))
	for i, w := range n.t.Words {
		parts[i] = lexeme(w)
	}
	return "(" + strings.Join(parts, " <-> ") + ")"
}
func (n andNode) ts() string { return "(" + n.l.ts() + " & " + n.r.ts() + ")" }
func (n orNode) ts() string  { return "(" + n.l.ts() + " | " + n.r.ts() + ")" }
func (n notNode) ts() string { return "!" + n.n.ts() }

// ts: l then r, or r then l, 1 to d words apart.
func (n nearNode) ts() string {
	parts := make([]string, 0, 2*n.d)
	for i := 1; i <= n.d; i++ {
		parts = append(parts, fmt.Sprintf("(%s <%d> %s)", n.l.ts(), i, n.r.ts()), fmt.Sprintf("(%s <%d> %s)", n.r.ts(), i, n.l.ts()))
	}
	return "(" + strings.Join(parts, " | ") + ")"
}

func (n termNode) collect(negated bool, add func(Term)) {
	if !negated {
		add(n.t)
	}
}
func (n andNode) collect(neg bool, add func(Term))  { n.l.collect(neg, add); n.r.collect(neg, add) }
func (n orNode) collect(neg bool, add func(Term))   { n.l.collect(neg, add); n.r.collect(neg, add) }
func (n notNode) collect(neg bool, add func(Term))  { n.n.collect(!neg, add) }
func (n nearNode) collect(neg bool, add func(Term)) { n.l.collect(neg, add); n.r.collect(neg, add) }
