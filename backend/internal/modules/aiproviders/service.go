package aiproviders

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the module's use cases.
type Service struct {
	pool     *pgxpool.Pool
	settings Settings
}

// Saved is a provider's settings as shown (never the key).
type Saved struct {
	Provider, Model, KeyHint string
	UpdatedAt                time.Time
	CheckStatus              *string
	CheckMessage             *string
	CheckedAt                *time.Time
}

// State is the company's AI providers: the one in use and what each has saved.
type State struct {
	Active          string
	ServerAvailable bool
	KeysSupported   bool
	Saved           map[string]Saved
}

func (s *Service) state(ctx context.Context, q *store.Queries) (State, error) {
	choice, err := choiceIn(ctx, q)
	if err != nil {
		return State{}, err
	}
	rows, err := q.ListProviders(ctx)
	if err != nil {
		return State{}, err
	}
	st := State{Active: choice, ServerAvailable: s.settings.ServerOn, KeysSupported: len(s.settings.KeysSecret) > 0, Saved: map[string]Saved{}}
	for _, r := range rows {
		st.Saved[r.Provider] = Saved{
			Provider: r.Provider, Model: r.Model, KeyHint: r.KeyHint, UpdatedAt: r.UpdatedAt,
			CheckStatus: r.CheckStatus, CheckMessage: r.CheckMessage, CheckedAt: r.CheckedAt,
		}
	}
	return st, nil
}

func choiceIn(ctx context.Context, q *store.Queries) (string, error) {
	choice, err := q.GetChoice(ctx)
	if database.IsNoRows(err) {
		return "server", nil
	}
	return choice, err
}

// Get returns the company's AI provider settings.
func (s *Service) Get(ctx context.Context, p authz.Principal) (State, error) {
	var out State
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.state(ctx, store.New(tx))
		return err
	})
	return out, err
}

// Choose switches the provider the company uses. A provider needs a saved key.
func (s *Service) Choose(ctx context.Context, p authz.Principal, choice string) (State, error) {
	var out State
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		before, err := choiceIn(ctx, q)
		if err != nil {
			return err
		}
		if choice != "server" && choice != "off" {
			if _, err := q.GetProvider(ctx, choice); database.IsNoRows(err) {
				return apperr.New(api.ErrorCodeAIPROVIDERNOKEY, "Save an API key for this provider first.")
			} else if err != nil {
				return err
			}
		}
		if err := q.SetChoice(ctx, choice); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "aiprovider.chosen", EntityType: "ai_provider", EntityID: choice,
			Changes: map[string]any{"provider": map[string]any{"from": before, "to": choice}},
		}); err != nil {
			return err
		}
		out, err = s.state(ctx, q)
		return err
	})
	return out, err
}

// Save keeps a provider's model and, when given, a new API key (sealed; the
// first save needs one), then queues a check of the two. A company without a
// provider in use yet switches to this one.
func (s *Service) Save(ctx context.Context, p authz.Principal, provider, model string, key *string) (State, error) {
	if len(s.settings.KeysSecret) == 0 {
		return State{}, apperr.New(api.ErrorCodeAIKEYSNOTSETUP,
			"This server cannot store AI keys yet: ask whoever runs it to set HP_AI_KEYS_SECRET.")
	}
	model, key, err := CheckInput(model, key)
	if err != nil {
		return State{}, err
	}
	var out State
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		old, err := q.GetProvider(ctx, provider)
		switch {
		case database.IsNoRows(err) && key == nil:
			var errs apperr.FieldErrors
			errs.Add("apiKey", "is required the first time")
			return errs.Err()
		case err != nil && !database.IsNoRows(err):
			return err
		}
		sealed, hint := old.SealedKey, old.KeyHint
		if key != nil {
			if sealed, err = Seal(rand.Reader, s.settings.KeysSecret, provider, *key); err != nil {
				return err
			}
			hint = Hint(*key)
		}
		if err := q.SaveProvider(ctx, store.SaveProviderParams{Provider: provider, Model: model, SealedKey: sealed, KeyHint: hint}); err != nil {
			return err
		}
		if err := queueCheck(ctx, tx, p.TenantID, provider, model, sealed); err != nil {
			return err
		}
		if choice, err := choiceIn(ctx, q); err != nil {
			return err
		} else if choice == "server" && !s.settings.ServerOn {
			if err := q.SetChoice(ctx, provider); err != nil {
				return err
			}
		}
		// Never the key, not even sealed: only whether it changed.
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "aiprovider.saved", EntityType: "ai_provider", EntityID: provider,
			Changes: map[string]any{"model": model, "keyChanged": key != nil},
		}); err != nil {
			return err
		}
		out, err = s.state(ctx, q)
		return err
	})
	return out, err
}

func queueCheck(ctx context.Context, tx pgx.Tx, tenantID, provider, model, sealed string) error {
	name := api.AiProviderName(provider)
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindAiproviderCheck, api.AiProviderCheckJob{
		Provider: name, Access: api.AiAccess{Provider: name, Model: model, SealedKey: sealed},
	})
	return err
}

// Remove deletes a provider's key; the AI is switched off if it was in use.
func (s *Service) Remove(ctx context.Context, p authz.Principal, provider string) (State, error) {
	var out State
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.DeleteProvider(ctx, provider)
		if err != nil {
			return err
		}
		if n > 0 {
			if choice, err := choiceIn(ctx, q); err != nil {
				return err
			} else if choice == provider {
				if err := q.SetChoice(ctx, "off"); err != nil {
					return err
				}
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: "aiprovider.removed", EntityType: "ai_provider", EntityID: provider}); err != nil {
				return err
			}
		}
		out, err = s.state(ctx, q)
		return err
	})
	return out, err
}

// Check queues a new check of a provider's saved key and model.
func (s *Service) Check(ctx context.Context, p authz.Principal, provider string) (State, error) {
	var out State
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetProvider(ctx, provider)
		if database.IsNoRows(err) {
			return apperr.New(api.ErrorCodeAIPROVIDERNOKEY, "Save an API key for this provider first.")
		} else if err != nil {
			return err
		}
		if err := q.MarkChecking(ctx, provider); err != nil {
			return err
		}
		if err := queueCheck(ctx, tx, p.TenantID, provider, row.Model, row.SealedKey); err != nil {
			return err
		}
		out, err = s.state(ctx, q)
		return err
	})
	return out, err
}

// checked handles aiprovider.checked: whether the provider answered.
func (s *Service) checked(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.AiProviderCheckedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	status := "ok"
	var message *string
	if !r.Ok {
		status = "failed"
		m := "The provider did not answer."
		if v, err := r.Message.Get(); err == nil && v != "" {
			m = v
		}
		if len(m) > 500 {
			m = m[:500]
		}
		message = &m
	}
	// A check of a key replaced since changes nothing.
	_, err := store.New(tx).SaveCheck(ctx, store.SaveCheckParams{
		Provider: string(r.Provider), SealedKey: r.SealedKey, CheckStatus: &status, CheckMessage: message,
	})
	return err
}

// access is the AI the company uses, for the modules that queue AI work.
func (s *Service) access(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error) {
	q := store.New(tx)
	choice, err := choiceIn(ctx, q)
	if err != nil {
		return nil, false, err
	}
	var entries []Entry
	if choice != "server" && choice != "off" {
		row, err := q.GetProvider(ctx, choice)
		if err != nil && !database.IsNoRows(err) {
			return nil, false, err
		}
		if err == nil {
			entries = append(entries, Entry{Provider: row.Provider, Model: row.Model, SealedKey: row.SealedKey})
		}
	}
	entry, on := Pick(choice, s.settings.ServerOn, entries)
	if entry == nil {
		return nil, on, nil
	}
	return &api.AiAccess{Provider: api.AiProviderName(entry.Provider), Model: entry.Model, SealedKey: entry.SealedKey}, true, nil
}
