package boolsearch

import "encoding/json"

// CandidatePlan serializes searches with NOT. Positive expressions are tested
// in a single profile or CV. NOT examines its operand in every source for the
// candidate, wherever it appears in the expression.
func (q Query) CandidatePlan() ([]byte, error) {
	if !hasNot(q.root) {
		return nil, nil
	}
	return json.Marshal(candidateNode(q.root))
}

func candidateNode(n node) any {
	if !hasNot(n) {
		return n.ts()
	}
	switch n := n.(type) {
	case notNode:
		return []any{"not", candidateNode(n.n)}
	case andNode:
		return []any{"and", candidateNode(n.l), candidateNode(n.r)}
	case orNode:
		return []any{"or", candidateNode(n.l), candidateNode(n.r)}
	default:
		return n.ts()
	}
}

func hasNot(n node) bool {
	switch n := n.(type) {
	case notNode:
		return true
	case andNode:
		return hasNot(n.l) || hasNot(n.r)
	case orNode:
		return hasNot(n.l) || hasNot(n.r)
	default:
		return false
	}
}

// CandidatePrefilter is a necessary match for a Boolean expression, with
// candidate-wide NOT branches treated as unconstrained. An empty result
// means the expression can match without a positive term.
func (q Query) CandidatePrefilter() string { return candidatePrefilter(q.root) }

func candidatePrefilter(n node) string {
	if !hasNot(n) {
		return n.ts()
	}
	switch n := n.(type) {
	case andNode:
		left, right := candidatePrefilter(n.l), candidatePrefilter(n.r)
		if left == "" {
			return right
		}
		if right == "" {
			return left
		}
		return "(" + left + " & " + right + ")"
	case orNode:
		left, right := candidatePrefilter(n.l), candidatePrefilter(n.r)
		if left == "" || right == "" {
			return ""
		}
		return "(" + left + " | " + right + ")"
	default:
		return ""
	}
}
