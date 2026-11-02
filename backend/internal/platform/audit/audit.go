// Package audit records who changed what. Call Record inside the same
// transaction as the change, so the trail can never miss a committed change
// or contain one that was rolled back.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/requestid"
)

// Entry describes one change. Action is entity.verb, e.g. candidate.updated.
type Entry struct {
	Action     string
	EntityType string
	EntityID   string
	// Changes maps field name to {"from": old, "to": new}. For sensitive
	// fields use Redacted so values never reach the audit log.
	Changes map[string]any
}

// Redacted marks a sensitive field as changed without storing its values.
var Redacted = map[string]any{"changed": true, "redacted": true}

// Record writes e to the audit log of the transaction's tenant.
func Record(ctx context.Context, tx pgx.Tx, e Entry) error {
	changes := e.Changes
	if changes == nil {
		changes = map[string]any{}
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	var reqID *string
	if id := requestid.From(ctx); id != "" {
		reqID = &id
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log (tenant_id, actor_id, action, entity_type, entity_id, changes, request_id)
		VALUES (app_current_tenant(), coalesce(app_current_user(), 'system'), $1, $2, $3, $4, $5)`,
		e.Action, e.EntityType, e.EntityID, data, reqID)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

// Diff returns {field: {"from": before, "to": after}} for every field whose
// value differs. Both maps should have the same keys.
func Diff(before, after map[string]any) map[string]any {
	out := map[string]any{}
	for k, b := range before {
		if a := after[k]; !reflect.DeepEqual(a, b) {
			out[k] = map[string]any{"from": b, "to": a}
		}
	}
	return out
}

// Logged is one entry read back from the audit log.
type Logged struct {
	ID        string
	ActorID   string
	Action    string
	Changes   map[string]any
	CreatedAt time.Time
}

// List returns the newest entries about one entity (at most limit), in the
// transaction's tenant.
func List(ctx context.Context, tx pgx.Tx, entityType, entityID string, limit int) ([]Logged, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, actor_id, action, changes, created_at FROM audit_log
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY created_at DESC, id DESC LIMIT $3`, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}
	defer rows.Close()
	var out []Logged
	for rows.Next() {
		var l Logged
		var raw []byte
		if err := rows.Scan(&l.ID, &l.ActorID, &l.Action, &raw, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("audit: %w", err)
		}
		if err := json.Unmarshal(raw, &l.Changes); err != nil {
			return nil, fmt.Errorf("audit: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
