// Package audit records who changed what. Call Record inside the same
// transaction as the change, so the trail can never miss a committed change
// or contain one that was rolled back.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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
