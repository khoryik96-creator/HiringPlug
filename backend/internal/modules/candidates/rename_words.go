package candidates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
)

// Fields a word from the company's Settings lists is kept in.
const (
	SkillsField         = "skills"
	CertificationsField = "certifications"
	SpecializationField = "specialization"
)

// RenameWord renames a word from the company's lists on every candidate who
// has it in field (ignoring case; binned candidates too), and says how many
// candidates changed. A candidate who already has the new word keeps it once.
// It runs in the caller's transaction, which records the change.
func (m *Module) RenameWord(ctx context.Context, tx pgx.Tx, field, from, to string) (int, error) {
	q := store.New(tx)
	if err := q.ShowBinnedWords(ctx, "on"); err != nil {
		return 0, err
	}
	var n int64
	var err error
	switch field {
	case SkillsField:
		n, err = q.RenameSkill(ctx, store.RenameSkillParams{FromWord: from, ToWord: to})
	case CertificationsField:
		n, err = q.RenameCertification(ctx, store.RenameCertificationParams{FromWord: from, ToWord: to})
	case SpecializationField:
		n, err = q.RenameSpecialization(ctx, store.RenameSpecializationParams{FromWord: from, ToWord: to})
	default:
		err = fmt.Errorf("candidates: no word field %q", field)
	}
	if err != nil {
		return 0, err
	}
	return int(n), q.ShowBinnedWords(ctx, "")
}
