package settings

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Rename is a word renamed in one of the company's lists: the records that
// hold From get To.
type Rename struct{ From, To string }

// renameLists are the word lists whose renames carry over to the records
// holding them (industries have their own catalogue, with ids).
var renameLists = []string{"specialization", "it_skill", "certification"}

// Renamer carries a renamed word over to the records holding it, in the
// save's transaction, and says how many changed.
type Renamer interface {
	RenameOption(ctx context.Context, tx pgx.Tx, list, from, to string) (int, error)
}

// RenameWith gives settings the module that holds the words (wired after it
// exists).
func (m *Module) RenameWith(r Renamer) { m.service.renamer = r }

// Renames finds the words renamed in a save of a word list: an option sent
// with its old word as its key and a new label. One word renamed into another
// renamed in the same save could go either way, so it is refused: rename one
// at a time.
func Renames(name string, in []OptionInput, saved []Option) ([]Rename, error) {
	if !slices.Contains(renameLists, name) {
		return nil, nil
	}
	had, kept := map[string]bool{}, map[string]bool{}
	for _, o := range saved {
		had[o.Key] = true
	}
	for _, o := range in {
		kept[strings.Join(strings.Fields(o.Label), " ")] = true
	}
	// Words sent under another word: renamed, unless a new option keeps the old word.
	var changed []Rename
	from := map[string]bool{}
	for _, o := range in {
		key, label := strings.TrimSpace(o.Key), strings.Join(strings.Fields(o.Label), " ")
		if key == "" || label == "" || key == label || !had[key] {
			continue // a new word or one unchanged
		}
		if from[strings.ToLower(key)] {
			return nil, renameOneAtATime(key, label)
		}
		from[strings.ToLower(key)] = true
		changed = append(changed, Rename{From: key, To: label})
	}
	var out []Rename
	for _, r := range changed {
		if !strings.EqualFold(r.From, r.To) && from[strings.ToLower(r.To)] {
			return nil, renameOneAtATime(r.From, r.To)
		}
		if !kept[r.From] {
			out = append(out, r)
		}
	}
	return out, nil
}

func renameOneAtATime(from, to string) error {
	return apperr.Validation(apperr.FieldErrors{{Field: "options",
		Message: fmt.Sprintf("%q to %q: another word is renamed to or from it in the same save; rename one at a time", from, to)}})
}
