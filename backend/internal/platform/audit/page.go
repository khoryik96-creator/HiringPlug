package audit

// Reading the whole company's audit log, newest first (the audit log page).

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Filter narrows the company's audit log; each field is optional.
type Filter struct {
	Actor      *string // a user id, an API key id or "system"
	Action     *string // one action ("candidate.updated")
	ActionKind *string // every action of a kind ("candidate": candidate.*)
	EntityType *string
	EntityID   *string
	From, To   *time.Time // From <= created_at < To
}

// After is where a page of the log ended: its last entry's time and id.
type After struct {
	At time.Time
	ID string
}

// Page returns at most limit entries of the transaction's tenant that f
// finds, newest first, after where the previous page ended (nil: from the
// newest); more says whether there are further entries. Row-level security
// leaves out entries about candidates the user may not see.
func Page(ctx context.Context, tx pgx.Tx, f Filter, after *After, limit int) (entries []Logged, more bool, err error) {
	var afterAt *time.Time
	var afterID string
	if after != nil {
		afterAt, afterID = &after.At, after.ID
	}
	rows, err := tx.Query(ctx, `
		SELECT id, actor_id, action, entity_type, entity_id, changes, created_at FROM audit_log
		WHERE ($1::text IS NULL OR actor_id = $1)
		  AND ($2::text IS NULL OR action = $2)
		  AND ($3::text IS NULL OR starts_with(action, $3 || '.'))
		  AND ($4::text IS NULL OR entity_type = $4)
		  AND ($5::text IS NULL OR entity_id = $5)
		  AND ($6::timestamptz IS NULL OR created_at >= $6)
		  AND ($7::timestamptz IS NULL OR created_at < $7)
		  AND ($8::timestamptz IS NULL OR (created_at, id) < ($8, $9::text))
		ORDER BY created_at DESC, id DESC LIMIT $10`,
		f.Actor, f.Action, f.ActionKind, f.EntityType, f.EntityID, f.From, f.To, afterAt, afterID, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("audit: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l Logged
		var raw []byte
		if err := rows.Scan(&l.ID, &l.ActorID, &l.Action, &l.EntityType, &l.EntityID, &raw, &l.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("audit: %w", err)
		}
		if err := json.Unmarshal(raw, &l.Changes); err != nil {
			return nil, false, fmt.Errorf("audit: %w", err)
		}
		entries = append(entries, l)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("audit: %w", err)
	}
	if len(entries) > limit {
		return entries[:limit], true, nil
	}
	return entries, false, nil
}
