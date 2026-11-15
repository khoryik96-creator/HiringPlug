package folders

// The folders rules, as plain functions on plain values: no database, no HTTP.

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Kinds of folder: what they hold.
const (
	Candidates = "candidate"
	Contacts   = "contact"
	Companies  = "company"
)

// Folder is one folder, as its owner or someone it is shared with sees it.
type Folder struct {
	ID, Kind, Name, OwnerID string
	Mine                    bool // the signed-in person owns it
	SharedWith              []string
	ItemCount               int
	Version                 int
	CreatedAt, UpdatedAt    time.Time
}

// NormalizeName trims and squeezes spaces, and checks the name is not blank
// or too long.
func NormalizeName(name string) (string, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		errs.Add("name", "is required")
	case len([]rune(name)) > 100:
		errs.Add("name", "must be at most 100 characters")
	}
	return name, errs
}

var itemID = map[string]*regexp.Regexp{
	Candidates: regexp.MustCompile(`^cand_[0-9a-hjkmnp-tv-z]{26}$`),
	Contacts:   regexp.MustCompile(`^cont_[0-9a-hjkmnp-tv-z]{26}$`),
	Companies:  regexp.MustCompile(`^comp_[0-9a-hjkmnp-tv-z]{26}$`),
}

// ItemIDs checks that ids are records of the folder's kind (a candidate
// folder holds candidates), and drops repeats.
func ItemIDs(kind string, ids []string) ([]string, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	re := itemID[kind]
	var out []string
	for _, id := range ids {
		if re == nil || !re.MatchString(id) {
			errs.Add("itemIds", "must be "+kind+" ids")
			return nil, errs
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, errs
}
