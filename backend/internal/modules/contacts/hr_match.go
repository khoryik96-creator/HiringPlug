package contacts

// Who in HyPlug a person Find HR found is, or may be: the rule that picks
// the likeliest contact among those the database offers (ContactsLike), as a
// pure function.

import (
	"slices"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/linkedin"
)

// HRMatchKind is how a contact matches a person: the same LinkedIn profile,
// the same name words in any order, or one name inside the other.
type HRMatchKind string

const (
	HRMatchProfile     HRMatchKind = "profile"
	HRMatchSameName    HRMatchKind = "same_name"
	HRMatchSimilarName HRMatchKind = "similar_name"
)

func (k HRMatchKind) rank() int {
	switch k {
	case HRMatchProfile:
		return 0
	case HRMatchSameName:
		return 1
	default:
		return 2
	}
}

// HRContact is a contact who is, or may be, a person a scan found.
type HRContact struct {
	ID, Name, CompanyID string
	Title, Email        *string
	EmailCheckStatus    *string
	Left                bool
	Kind                HRMatchKind
	UpdatedAt           time.Time
}

// BestHRMatch is the contact likeliest to be the person: the same LinkedIn
// profile before the same name, and that before a similar one; then one at the
// company searched, one who still works there, one with an email, the one
// saved last. more counts the others.
func BestHRMatch(found []HRContact, companyID string) (best HRContact, more int, ok bool) {
	if len(found) == 0 {
		return HRContact{}, 0, false
	}
	ordered := slices.Clone(found)
	slices.SortStableFunc(ordered, func(a, b HRContact) int {
		if a.Kind.rank() != b.Kind.rank() {
			return a.Kind.rank() - b.Kind.rank()
		}
		for _, first := range [][2]bool{
			{a.CompanyID == companyID, b.CompanyID == companyID},
			{!a.Left, !b.Left},
			{a.Email != nil, b.Email != nil},
		} {
			if first[0] != first[1] {
				if first[0] {
					return -1
				}
				return 1
			}
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return ordered[0], len(ordered) - 1, true
}

// hrProfileKey is a LinkedIn profile as ContactsLike compares it: tidied as
// contacts are saved, in lower case (LinkedIn's addresses ignore case).
func hrProfileKey(url string) string {
	if normal, ok := linkedin.Normalize(url); ok {
		url = normal
	}
	return strings.ToLower(strings.TrimRight(url, "/"))
}
