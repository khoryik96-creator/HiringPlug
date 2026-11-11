package industries

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// CheckEdit enforces a two-level tree and unambiguous canonical labels.
func CheckEdit(items []api.Industry, id string, in api.IndustryEdit) (api.IndustryEdit, error) {
	in.Label = strings.Join(strings.Fields(in.Label), " ")
	if n := utf8.RuneCountInString(in.Label); n < 1 || n > 100 {
		return in, invalid("label", "must contain 1–100 characters")
	}
	byID := catalogueMap(items)
	if id != "" {
		if _, ok := byID[id]; !ok {
			return in, apperr.NotFound("industry")
		}
	}
	if in.ParentId != nil {
		parent, ok := byID[*in.ParentId]
		if !ok || parent.ParentId != nil || parent.Id == id {
			return in, invalid("parentId", "choose a main industry; only two levels are supported")
		}
		if parent.Retired && !in.Retired {
			return in, invalid("parentId", "choose an active main industry")
		}
		for _, item := range items {
			if item.ParentId != nil && *item.ParentId == id {
				return in, invalid("parentId", "an industry with subindustries cannot become a subindustry")
			}
		}
	}
	for _, item := range items {
		if item.Id != id && Key(item.Label) == Key(in.Label) {
			return in, invalid("label", "this category already exists")
		}
	}
	aliases := []string{}
	seen := map[string]bool{}
	for _, alias := range in.Aliases {
		alias = strings.Join(strings.Fields(alias), " ")
		k := Key(alias)
		if alias == "" || utf8.RuneCountInString(alias) > 100 {
			return in, invalid("aliases", "each alias must contain 1–100 characters")
		}
		if k != Key(in.Label) && !seen[k] {
			aliases = append(aliases, alias)
			seen[k] = true
		}
	}
	if len(aliases) > 30 {
		return in, invalid("aliases", "use at most 30 aliases")
	}
	in.Aliases = aliases
	return in, nil
}

func catalogueMap(items []api.Industry) map[string]api.Industry {
	m := map[string]api.Industry{}
	for _, i := range items {
		m[i.Id] = i
	}
	return m
}
func active(i api.Industry, items map[string]api.Industry) bool {
	if i.Retired {
		return false
	}
	return i.ParentId == nil || !items[*i.ParentId].Retired
}
func root(i api.Industry, items map[string]api.Industry) api.Industry {
	if i.ParentId != nil {
		return items[*i.ParentId]
	}
	return i
}

// Match accepts unique normalized canonical labels or aliases; ambiguity is never guessed.
func Match(items []api.Industry, value string) *api.Industry {
	if Key(value) == "" {
		return nil
	}
	var found *api.Industry
	for _, i := range items {
		matches := Key(i.Label) == Key(value) || slices.ContainsFunc(i.Aliases, func(a string) bool { return Key(a) == Key(value) })
		if matches {
			if found != nil {
				return nil
			}
			copy := i
			found = &copy
		}
	}
	return found
}
