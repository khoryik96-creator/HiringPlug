package automations

// The automations and the company's default: one transaction each.

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the module's use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	now  func() time.Time
}

// View is an automation as shown.
type View struct {
	store.Automation
	Rule        Rule
	CompanyName *string
}

// List returns the company's automations and its default for new ones; the
// first time, it adds the ready-made ones (switched off).
func (s *Service) List(ctx context.Context, p authz.Principal) ([]View, string, error) {
	var out []View
	var mode string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		settings, err := q.EnsureSettings(ctx)
		if err != nil {
			return err
		}
		mode = settings.DefaultSendMode
		if n, err := q.MarkSeeded(ctx); err != nil {
			return err
		} else if n == 1 {
			for _, preset := range Presets(mode) {
				key := preset.Key
				if _, err := insert(ctx, q, preset.Rule, &key); err != nil {
					return err
				}
			}
		}
		rows, err := q.ListAutomations(ctx)
		if err != nil {
			return err
		}
		sortPresetsFirst(rows)
		out, err = s.views(ctx, tx, rows)
		return err
	})
	return out, mode, err
}

// Create adds an automation.
func (s *Service) Create(ctx context.Context, p authz.Principal, r Rule) (View, error) {
	r, errs := NormalizeRule(r)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.checkCompany(ctx, tx, r.CompanyID); err != nil {
			return err
		}
		row, err := insert(ctx, store.New(tx), r, nil)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "automation.created", EntityType: "automation", EntityID: row.ID, Changes: summary(r)}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, row)
		return err
	})
	return out, err
}

// Update changes an automation, if nobody changed it since version.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, version int, r Rule) (View, error) {
	r, errs := NormalizeRule(r)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	actions, err := json.Marshal(actionsJSON(r.Actions))
	if err != nil {
		return View{}, err
	}
	var out View
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetAutomation(ctx, id); database.IsNoRows(err) {
			return apperr.NotFound("automation")
		} else if err != nil {
			return err
		}
		if err := s.checkCompany(ctx, tx, r.CompanyID); err != nil {
			return err
		}
		row, err := q.UpdateAutomation(ctx, store.UpdateAutomationParams{
			ID: id, ExpectedVersion: int32(version), Name: r.Name, Enabled: r.Enabled, Trigger: r.Trigger, Stage: r.Stage, //nolint:gosec // versions are small
			OffsetDays: int32(r.OffsetDays), CompanyID: r.CompanyID, Actions: actions, SendMode: r.SendMode, //nolint:gosec // -60..365
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("automation")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "automation.updated", EntityType: "automation", EntityID: id, Changes: summary(r)}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, row)
		return err
	})
	return out, err
}

// Delete removes an automation and what it has scheduled.
func (s *Service) Delete(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		n, err := store.New(tx).DeleteAutomation(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("automation")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "automation.deleted", EntityType: "automation", EntityID: id})
	})
}

// SetDefault sets whether new automations send on their own or ask first.
func (s *Service) SetDefault(ctx context.Context, p authz.Principal, mode string) (string, error) {
	if mode != SendOnTheirOwn && mode != AskFirst {
		var errs apperr.FieldErrors
		errs.Add("defaultSendMode", "must be send or approve")
		return "", errs.Err()
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.EnsureSettings(ctx); err != nil {
			return err
		}
		if _, err := q.PutSettings(ctx, mode); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "automation.default_set", EntityType: "tenant", EntityID: p.TenantID,
			Changes: map[string]any{"defaultSendMode": mode}})
	})
	return mode, err
}

// sortPresetsFirst lists the ready-made automations in their own order (they
// are added at the same moment), then the company's own, oldest first.
func sortPresetsFirst(rows []store.Automation) {
	order := map[string]int{}
	for i, p := range Presets(AskFirst) {
		order[p.Key] = i
	}
	rank := func(r store.Automation) int {
		if r.Preset != nil {
			if i, ok := order[*r.Preset]; ok {
				return i
			}
		}
		return len(order)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rank(rows[i]) < rank(rows[j]) })
}

func insert(ctx context.Context, q *store.Queries, r Rule, preset *string) (store.Automation, error) {
	actions, err := json.Marshal(actionsJSON(r.Actions))
	if err != nil {
		return store.Automation{}, err
	}
	return q.InsertAutomation(ctx, store.InsertAutomationParams{
		Name: r.Name, Enabled: r.Enabled, Trigger: r.Trigger, Stage: r.Stage, OffsetDays: int32(r.OffsetDays), //nolint:gosec // -60..365
		CompanyID: r.CompanyID, Actions: actions, SendMode: r.SendMode, Preset: preset,
	})
}

func (s *Service) checkCompany(ctx context.Context, tx pgx.Tx, id *string) error {
	if id == nil {
		return nil
	}
	names, err := s.deps.Companies.Names(ctx, tx, []string{*id})
	if err != nil {
		return err
	}
	if _, ok := names[*id]; !ok {
		var errs apperr.FieldErrors
		errs.Add("companyId", "no such client")
		return errs.Err()
	}
	return nil
}

func (s *Service) one(ctx context.Context, tx pgx.Tx, row store.Automation) (View, error) {
	views, err := s.views(ctx, tx, []store.Automation{row})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

func (s *Service) views(ctx context.Context, tx pgx.Tx, rows []store.Automation) ([]View, error) {
	ids := []string{}
	for _, r := range rows {
		if r.CompanyID != nil {
			ids = append(ids, *r.CompanyID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		var err error
		if names, err = s.deps.Companies.Names(ctx, tx, ids); err != nil {
			return nil, err
		}
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		rule, err := ruleOf(r)
		if err != nil {
			return nil, err
		}
		out[i] = View{Automation: r, Rule: rule}
		if r.CompanyID != nil {
			if n, ok := names[*r.CompanyID]; ok {
				out[i].CompanyName = &n
			}
		}
	}
	return out, nil
}

// actionJSON is how an action is stored.
type actionJSON struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
	Title   string `json:"title,omitempty"`
}

func actionsJSON(in []Action) []actionJSON {
	out := make([]actionJSON, len(in))
	for i, a := range in {
		out[i] = actionJSON(a)
	}
	return out
}

func ruleOf(r store.Automation) (Rule, error) {
	var actions []actionJSON
	if err := json.Unmarshal(r.Actions, &actions); err != nil {
		return Rule{}, err
	}
	out := Rule{Name: r.Name, Enabled: r.Enabled, Trigger: r.Trigger, Stage: r.Stage, OffsetDays: int(r.OffsetDays),
		CompanyID: r.CompanyID, SendMode: r.SendMode, Actions: make([]Action, len(actions))}
	for i, a := range actions {
		out.Actions[i] = Action(a)
	}
	return out, nil
}

// summary is what the audit log keeps of a change (the texts stay in the automation).
func summary(r Rule) map[string]any {
	kinds := make([]string, len(r.Actions))
	for i, a := range r.Actions {
		kinds[i] = a.Kind
	}
	return map[string]any{"name": r.Name, "enabled": r.Enabled, "trigger": r.Trigger, "stage": r.Stage, "offsetDays": r.OffsetDays,
		"companyId": r.CompanyID, "actions": kinds, "sendMode": r.SendMode}
}
