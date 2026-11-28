package settings

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the settings use cases.
type Service struct {
	pool       *pgxpool.Pool
	industries IndustryCatalogue
	uses       Uses
	renamer    Renamer
	backups    BackupSetup
	paidOff    bool // HP_AI_PAID=off
	server     ServerSetup
}

// Settings is every dropdown and every field's mandatory choice.
type Settings struct {
	Lists  []List
	Fields []MandatoryField
}

func listsIn(ctx context.Context, q *store.Queries) ([]List, error) {
	saved, err := q.SavedLists(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOptions(ctx)
	if err != nil {
		return nil, err
	}
	options := map[string][]Option{}
	for _, name := range saved {
		options[name] = []Option{}
	}
	for _, r := range rows {
		options[r.List] = append(options[r.List], Option{Key: r.Key, Label: r.Label, Hint: deref(r.Hint), Color: deref(r.Color), Hidden: r.Hidden})
	}
	return Lists(options), nil
}

func rulesIn(ctx context.Context, q *store.Queries) (map[string]bool, error) {
	rows, err := q.ListRules(ctx)
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.Form+"/"+r.Field] = r.Mandatory
	}
	return out, err
}

func settingsIn(ctx context.Context, q *store.Queries) (Settings, error) {
	lists, err := listsIn(ctx, q)
	if err != nil {
		return Settings{}, err
	}
	rules, err := rulesIn(ctx, q)
	return Settings{Lists: lists, Fields: Fields(rules)}, err
}

// Get returns the company's settings.
func (s *Service) Get(ctx context.Context, p authz.Principal) (Settings, error) {
	var out Settings
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = settingsIn(ctx, store.New(tx))
		if err != nil {
			return err
		}
		return s.industryList(ctx, tx, &out)
	})
	return out, err
}

// PutList replaces one dropdown's options. A word renamed in the
// specialization, IT skills or certifications list (sent with its old word as
// its key) is renamed on the candidates who have it too. basedOn, when given,
// are the keys the editor loaded: a list changed since is not overwritten.
func (s *Service) PutList(ctx context.Context, p authz.Principal, name string, in []OptionInput, basedOn []string) (List, error) {
	if name == "industry" {
		return List{}, invalidIndustryList()
	}
	options, err := CheckList(name, in)
	if err != nil {
		return List{}, err
	}
	var out List
	var renames []Rename
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		before, err := listIn(ctx, q, name)
		if err != nil {
			return err
		}
		if basedOn != nil && !sameKeys(before.Options, basedOn) {
			return apperr.StaleVersion("list")
		}
		renames, err = Renames(name, in, before.Options)
		if err != nil {
			return err
		}
		if err := s.checkRemovedUnused(ctx, tx, name, before.Options, options); err != nil {
			return err
		}
		if err := q.DeleteOptions(ctx, name); err != nil {
			return err
		}
		for i, o := range options {
			if err := q.InsertOption(ctx, store.InsertOptionParams{List: name, Key: o.Key, Label: o.Label, Hint: nonEmpty(o.Hint), Color: nonEmpty(o.Color), Hidden: o.Hidden, Position: int32(i)}); err != nil { //nolint:gosec // at most 200
				return err
			}
		}
		if err := q.MarkSaved(ctx, name); err != nil {
			return err
		}
		changes := map[string]any{"options": len(options)}
		if renamed, err := s.carryOver(ctx, tx, name, renames); err != nil {
			return err
		} else if len(renamed) > 0 {
			changes["renamed"] = renamed
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "settings.list_changed", EntityType: "field_list", EntityID: name, Changes: changes,
		}); err != nil {
			return err
		}
		out, err = listIn(ctx, q, name)
		return err
	})
	if err == nil && len(renames) > 0 && s.renamer != nil {
		s.renamer.RenamesSaved() // what was remembered of the old words, now committed
	}
	return out, err
}

// sameKeys is whether a list has exactly these keys (in any order).
func sameKeys(options []Option, keys []string) bool {
	if len(options) != len(keys) {
		return false
	}
	have := make(map[string]bool, len(options))
	for _, o := range options {
		have[o.Key] = true
	}
	for _, k := range keys {
		if !have[k] {
			return false
		}
	}
	return true
}

// listIn is one of the company's dropdowns as saved now.
func listIn(ctx context.Context, q *store.Queries, name string) (List, error) {
	lists, err := listsIn(ctx, q)
	for _, l := range lists {
		if l.Name == name {
			return l, err
		}
	}
	return List{}, err
}

// carryOver renames the words on the records holding them, and says what was
// renamed and on how many.
func (s *Service) carryOver(ctx context.Context, tx pgx.Tx, name string, renames []Rename) ([]map[string]any, error) {
	if s.renamer == nil {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(renames))
	for _, r := range renames {
		n, err := s.renamer.RenameOption(ctx, tx, name, r.From, r.To)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"from": r.From, "to": r.To, "records": n})
	}
	return out, nil
}

// checkRemovedUnused refuses to remove an option records still hold (a
// contact's role, a company's category): they would show a bare key. Hiding
// it keeps it for them and out of new choices.
func (s *Service) checkRemovedUnused(ctx context.Context, tx pgx.Tx, name string, before, options []Option) error {
	if s.uses == nil {
		return nil
	}
	for _, old := range before {
		if slices.ContainsFunc(options, func(o Option) bool { return o.Key == old.Key }) {
			continue
		}
		n, err := s.uses.OptionUses(ctx, tx, name, old.Key)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.Validation(apperr.FieldErrors{{Field: "options", Message: fmt.Sprintf("%q is still used by %d %s: hide it instead of removing it", old.Label, n, usedBy(name, n))}})
		}
	}
	return nil
}

func usedBy(list string, n int) string {
	what := map[string]string{"contact_role": "contact", "company_category": "company", "claim_receipt_type": "claim", "claim_type": "claim"}[list]
	if what == "" {
		what = "record"
	}
	if n == 1 {
		return what
	}
	if what == "company" {
		return "companies"
	}
	return what + "s"
}

// PutRules sets which fields are mandatory.
func (s *Service) PutRules(ctx context.Context, p authz.Principal, in []RuleInput) (Settings, error) {
	if err := CheckRules(in); err != nil {
		return Settings{}, err
	}
	var out Settings
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		changes := map[string]any{}
		for _, r := range in {
			if err := q.PutRule(ctx, store.PutRuleParams{Form: r.Form, Field: r.Field, Mandatory: r.Mandatory}); err != nil {
				return err
			}
			changes[r.Form+"."+r.Field] = r.Mandatory
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "settings.mandatory_changed", EntityType: "field_rules", EntityID: "fields", Changes: changes,
		}); err != nil {
			return err
		}
		var err error
		out, err = settingsIn(ctx, q)
		return err
	})
	return out, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
