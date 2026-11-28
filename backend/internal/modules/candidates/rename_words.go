package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
)

// Fields a word from the company's Settings lists is kept in.
const (
	SkillsField         = "skills"
	CertificationsField = "certifications"
	SpecializationField = "specialization"
)

// RenameWord renames a word from the company's lists on every candidate of
// the company who has it in field (ignoring case; whoever owns them, binned
// ones too), records the change on each, and says how many changed. A
// candidate who already has the new word keeps it once. It runs in the
// caller's transaction, for someone who manages Settings (the database
// checks).
func (m *Module) RenameWord(ctx context.Context, tx pgx.Tx, field, from, to string) (int, error) {
	ids, err := store.New(tx).RenameCandidateWord(ctx, store.RenameCandidateWordParams{Field: field, FromWord: from, ToWord: to})
	if err != nil {
		return 0, err
	}
	entries := make([]audit.Entry, len(ids))
	for i, id := range ids {
		entries[i] = audit.Entry{Action: "candidate.updated", EntityType: "candidate", EntityID: id,
			Changes: map[string]any{field: map[string]any{"from": from, "to": to}, "renamedInList": true}}
	}
	if err := audit.RecordAll(ctx, tx, entries); err != nil {
		return 0, err
	}
	if len(ids) > 0 {
		m.service.used.forgetAll() // the suggestions have the old word
	}
	return len(ids), nil
}
