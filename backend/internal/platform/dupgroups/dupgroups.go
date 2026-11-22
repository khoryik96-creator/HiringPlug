// Package dupgroups joins records found alike for different reasons (the
// same website, the same name...) into groups of possible duplicates.
package dupgroups

import (
	"cmp"
	"slices"
)

// Group is records found alike for one reason (the same website, name...).
type Group struct {
	Reason string
	IDs    []string
}

// Joined is records found alike for one or more reasons.
type Joined struct {
	Reasons []string
	IDs     []string // newest first
}

// Join joins groups that share a record (A and B have the same
// website, B and C the same name: one group of three), newest first.
func Join(groups []Group) []Joined {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			r := find(p)
			parent[x] = r
			return r
		}
		parent[x] = x
		return x
	}
	for _, g := range groups {
		for _, id := range g.IDs[1:] {
			a, b := find(g.IDs[0]), find(id)
			if a != b {
				parent[b] = a
			}
		}
	}
	byRoot := map[string]*Joined{}
	var roots []string
	for _, g := range groups {
		r := find(g.IDs[0])
		j, ok := byRoot[r]
		if !ok {
			j = &Joined{}
			byRoot[r] = j
			roots = append(roots, r)
		}
		if !slices.Contains(j.Reasons, g.Reason) {
			j.Reasons = append(j.Reasons, g.Reason)
		}
		for _, id := range g.IDs {
			if !slices.Contains(j.IDs, id) {
				j.IDs = append(j.IDs, id)
			}
		}
	}
	out := make([]Joined, 0, len(roots))
	for _, r := range roots {
		j := *byRoot[r]
		slices.Sort(j.Reasons)
		slices.Sort(j.IDs)
		slices.Reverse(j.IDs) // ids sort by creation time
		out = append(out, j)
	}
	slices.SortFunc(out, func(a, b Joined) int { return cmp.Compare(b.IDs[0], a.IDs[0]) })
	return out
}
