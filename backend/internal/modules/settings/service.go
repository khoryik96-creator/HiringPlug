package settings

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the settings use cases.
type Service struct{ pool *pgxpool.Pool }

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
		return err
	})
	return out, err
}

// PutList replaces one dropdown's options.
func (s *Service) PutList(ctx context.Context, p authz.Principal, name string, in []OptionInput) (List, error) {
	options, err := CheckList(name, in)
	if err != nil {
		return List{}, err
	}
	var out List
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
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
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "settings.list_changed", EntityType: "field_list", EntityID: name, Changes: map[string]any{"options": len(options)},
		}); err != nil {
			return err
		}
		lists, err := listsIn(ctx, q)
		for _, l := range lists {
			if l.Name == name {
				out = l
			}
		}
		return err
	})
	return out, err
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
