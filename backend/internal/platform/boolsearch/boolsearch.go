// Package boolsearch reads a recruiter's boolean search, the way job boards
// take it: SAP AND (BW OR BPC) NOT internship, "project manager", java*.
//
//   - words next to each other must all appear (AND is implied);
//   - OR between alternatives; NOT (or -word) leaves out;
//   - ( ) groups; "quoted words" must appear together, in that order;
//   - word* is any word starting so (manag* finds manager, managing).
//
// NOT binds tightest, then AND, then OR. The operators are read in any case
// (and, or, not). The query becomes a PostgreSQL text search query
// (to_tsquery('english', ...)), so words match their other forms
// (manager, managers) as CV text is indexed.
package boolsearch

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Limits on a query (the contract allows 1000 characters).
const (
	MaxTerms = 50
	MaxDepth = 10
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
)

type token struct {
	kind   tokKind
	text   string
	prefix bool
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
		case r == '-' && (i == 0 || unicode.IsSpace(rs[i-1]) || rs[i-1] == '(') && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]):
			out = append(out, token{kind: tokNot, text: "-"}) // -internship
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
	left, err := p.unary(depth)
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
		right, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
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
		inner, err := p.unary(depth)
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

func (n termNode) collect(negated bool, add func(Term)) {
	if !negated {
		add(n.t)
	}
}
func (n andNode) collect(neg bool, add func(Term)) { n.l.collect(neg, add); n.r.collect(neg, add) }
func (n orNode) collect(neg bool, add func(Term))  { n.l.collect(neg, add); n.r.collect(neg, add) }
func (n notNode) collect(neg bool, add func(Term)) { n.n.collect(!neg, add) }
