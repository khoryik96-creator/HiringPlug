package identity

// A person's favourites in the dropdowns (specialization, industries, IT
// skills, certifications, countries): shown first for them only. They never
// change the lists themselves.

import (
	"encoding/json"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// cleanFavourites trims each kind's items and drops blanks and repeats (the
// same word in another case counts once), keeping the order chosen. Every
// kind is present in the result (empty when none).
func cleanFavourites(in api.Favourites) api.Favourites {
	words := func(v *[]string) *[]string {
		out := []string{}
		seen := map[string]bool{}
		if v != nil {
			for _, s := range *v {
				s = strings.TrimSpace(s)
				if k := strings.ToLower(s); s != "" && !seen[k] {
					seen[k] = true
					out = append(out, s)
				}
			}
		}
		return &out
	}
	return api.Favourites{
		Specialization: words(in.Specialization),
		ItSkill:        words(in.ItSkill),
		Certification:  words(in.Certification),
		Industry:       words(in.Industry),
		Country:        words(in.Country),
	}
}

// mergeFavourites puts the kinds sent over those saved (a kind left out keeps
// its favourites), so saving one kind never touches another.
func mergeFavourites(saved, sent api.Favourites) api.Favourites {
	pick := func(s, n *[]string) *[]string {
		if n != nil {
			return n
		}
		return s
	}
	return cleanFavourites(api.Favourites{
		Specialization: pick(saved.Specialization, sent.Specialization),
		ItSkill:        pick(saved.ItSkill, sent.ItSkill),
		Certification:  pick(saved.Certification, sent.Certification),
		Industry:       pick(saved.Industry, sent.Industry),
		Country:        pick(saved.Country, sent.Country),
	})
}

// favouritesOf reads the saved favourites; anything unreadable is none.
func favouritesOf(raw []byte) api.Favourites {
	var f api.Favourites
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &f) // written by SetPreferences only; a bad value reads as none
	}
	return cleanFavourites(f)
}
