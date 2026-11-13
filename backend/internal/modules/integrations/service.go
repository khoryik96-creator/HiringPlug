package integrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the integration use cases.
type Service struct {
	pool         *pgxpool.Pool
	users        Users
	allowPrivate bool
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system's random source failing is not recoverable
	}
	return hex.EncodeToString(b)
}

// inTx runs fn for a user who may manage integrations; API keys may not.
func (s *Service) inTx(ctx context.Context, p authz.Principal, fn func(tx pgx.Tx, q *store.Queries) error) error {
	if IsKeyPrincipal(p.UserID) {
		return apperr.New(api.ErrorCodeFORBIDDEN, "API keys cannot manage API keys or webhooks.")
	}
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error { return fn(tx, store.New(tx)) })
}

// APIKeys returns the company's keys that are not revoked.
func (s *Service) APIKeys(ctx context.Context, p authz.Principal) ([]store.ApiKey, map[string]string, error) {
	var keys []store.ApiKey
	var names map[string]string
	err := s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		if keys, err = q.ListApiKeys(ctx); err != nil {
			return err
		}
		ids := make([]string, len(keys))
		for i, k := range keys {
			ids[i] = k.CreatedBy
		}
		names, err = s.users.UserNames(ctx, tx, ids)
		return err
	})
	return keys, names, err
}

// CreateAPIKey makes a key acting as role; the key itself is returned only now.
func (s *Service) CreateAPIKey(ctx context.Context, p authz.Principal, name, role string) (store.ApiKey, string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return store.ApiKey{}, "", apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "is required"}})
	}
	if !MayGiveKeyRole(string(p.Role), role) {
		return store.ApiKey{}, "", apperr.New(api.ErrorCodeFORBIDDEN, "Only an admin or owner can make a key that acts as an admin.")
	}
	key := KeyPrefix + randomHex(32)
	var out store.ApiKey
	err := s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		row, err := q.InsertApiKey(ctx, store.InsertApiKeyParams{Name: name, Role: role, KeyHash: HashKey(key), Prefix: key[:len(KeyPrefix)+4]})
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{Action: "api_key.created", EntityType: "api_key", EntityID: row.ID, Changes: map[string]any{"name": name, "role": role}})
	})
	return out, key, err
}

// RevokeAPIKey stops a key working.
func (s *Service) RevokeAPIKey(ctx context.Context, p authz.Principal, id string) error {
	return s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		n, err := q.RevokeApiKey(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("API key")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "api_key.revoked", EntityType: "api_key", EntityID: id})
	})
}

// Webhooks returns the company's webhooks.
func (s *Service) Webhooks(ctx context.Context, p authz.Principal) ([]store.Webhook, error) {
	var out []store.Webhook
	err := s.inTx(ctx, p, func(_ pgx.Tx, q *store.Queries) error {
		var err error
		out, err = q.ListWebhooks(ctx)
		return err
	})
	return out, err
}

// CreateWebhook adds a webhook; its signing secret is returned only now.
func (s *Service) CreateWebhook(ctx context.Context, p authz.Principal, h Hook) (store.Webhook, string, error) {
	h, errs := NormalizeHook(h, s.allowPrivate)
	if err := errs.Err(); err != nil {
		return store.Webhook{}, "", err
	}
	secret := "whsec_" + randomHex(24)
	var out store.Webhook
	err := s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		row, err := q.InsertWebhook(ctx, store.InsertWebhookParams{Url: h.URL, Events: h.Events, Secret: secret, Description: h.Description})
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{Action: "webhook.created", EntityType: "webhook", EntityID: row.ID, Changes: map[string]any{"url": h.URL, "events": h.Events}})
	})
	return out, secret, err
}

// HookPatch is a partial change of a webhook.
type HookPatch struct {
	URL         *string
	Events      *[]string
	Active      *bool
	Description *string
	SetDesc     bool
}

// UpdateWebhook changes a webhook, if nobody changed it since version.
func (s *Service) UpdateWebhook(ctx context.Context, p authz.Principal, id string, version int, patch HookPatch) (store.Webhook, error) {
	var out store.Webhook
	err := s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		row, err := q.GetWebhook(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("webhook")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != version {
			return apperr.StaleVersion("webhook")
		}
		h := Hook{URL: row.Url, Events: row.Events, Active: row.Active, Description: row.Description}
		if patch.URL != nil {
			h.URL = *patch.URL
		}
		if patch.Events != nil {
			h.Events = *patch.Events
		}
		if patch.Active != nil {
			h.Active = *patch.Active
		}
		if patch.SetDesc {
			h.Description = patch.Description
		}
		h, errs := NormalizeHook(h, s.allowPrivate)
		if err := errs.Err(); err != nil {
			return err
		}
		out, err = q.UpdateWebhook(ctx, store.UpdateWebhookParams{
			ID: id, ExpectedVersion: int32(version), Url: h.URL, Events: h.Events, Active: h.Active, Description: h.Description, //nolint:gosec // versions are small
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("webhook")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "webhook.updated", EntityType: "webhook", EntityID: id,
			Changes: audit.Diff(map[string]any{"url": row.Url, "events": row.Events, "active": row.Active}, map[string]any{"url": h.URL, "events": h.Events, "active": h.Active})})
	})
	return out, err
}

// DeleteWebhook removes a webhook and its deliveries.
func (s *Service) DeleteWebhook(ctx context.Context, p authz.Principal, id string) error {
	return s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		n, err := q.DeleteWebhook(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("webhook")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "webhook.deleted", EntityType: "webhook", EntityID: id})
	})
}

// Deliveries returns a webhook's latest deliveries.
func (s *Service) Deliveries(ctx context.Context, p authz.Principal, id string) ([]store.WebhookDelivery, error) {
	var out []store.WebhookDelivery
	err := s.inTx(ctx, p, func(_ pgx.Tx, q *store.Queries) error {
		if _, err := q.GetWebhook(ctx, id); database.IsNoRows(err) {
			return apperr.NotFound("webhook")
		} else if err != nil {
			return err
		}
		var err error
		out, err = q.ListDeliveries(ctx, id)
		return err
	})
	return out, err
}

// Ping queues a webhook.ping delivery to one webhook, to check the receiver.
func (s *Service) Ping(ctx context.Context, p authz.Principal, id string) (store.WebhookDelivery, error) {
	var out store.WebhookDelivery
	err := s.inTx(ctx, p, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetWebhook(ctx, id); database.IsNoRows(err) {
			return apperr.NotFound("webhook")
		} else if err != nil {
			return err
		}
		deliveryID, err := q.NewDeliveryID(ctx)
		if err != nil {
			return err
		}
		event := string(api.WebhookEventWebhookPing)
		payload, err := json.Marshal(map[string]any{
			"id": deliveryID, "event": event, "occurredAt": time.Now().UTC(), "actorId": p.UserID,
			"entity": map[string]string{"type": "webhook", "id": id}, "changes": map[string]any{},
		})
		if err != nil {
			return err
		}
		out, err = q.InsertDelivery(ctx, store.InsertDeliveryParams{ID: deliveryID, WebhookID: id, Event: event, Payload: payload})
		if err != nil {
			return err
		}
		_, err = jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindWebhookDeliver, api.WebhookDeliverJob{DeliveryId: deliveryID})
		return err
	})
	return out, err
}
