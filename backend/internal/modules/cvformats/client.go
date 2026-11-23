package cvformats

// Which formatted CVs may go to a client (a client shortlist link): one
// HiringPlug made, without the salary, showing neither the candidate's
// email nor phone.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// FormattedFile is one of a candidate's documents, with its file (documents module).
type FormattedFile struct {
	CandidateID, Source string
	Data                []byte
}

// ClientCV is a formatted CV that may go to a client.
type ClientCV struct {
	Blind bool
	// Reference names the candidate when the CV is blind: "Candidate C-4F2K9Q".
	Reference string
}

// ForClient checks that a candidate's document may go to a client, in the
// caller's transaction. When it may not, reason says why, in words for the
// recruiter ("includes the salary: ...").
func (m *Module) ForClient(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID, documentID string) (ClientCV, string, error) {
	const notMade = "is not a formatted CV made by HiringPlug: make a blind or formatted CV for them first"
	f, err := m.svc.deps.Files.FormattedFile(ctx, tx, documentID)
	if err != nil {
		return ClientCV{}, "", err
	}
	if f.CandidateID != candidateID || f.Source != "formatted" {
		return ClientCV{}, notMade, nil
	}
	made, err := store.New(tx).GetFormattedCv(ctx, documentID)
	if database.IsNoRows(err) {
		return ClientCV{}, notMade, nil
	}
	if err != nil {
		return ClientCV{}, "", err
	}
	if made.IncludeSalary {
		return ClientCV{}, "includes the salary: make one without it", nil
	}
	cand, err := m.svc.deps.Candidates.ForCV(ctx, tx, p, candidateID)
	if err != nil {
		return ClientCV{}, "", err
	}
	leaks, err := Leaks(f.Data, IdentityOf(cand))
	if err != nil {
		return ClientCV{}, notMade, nil //nolint:nilerr // not a Word file HiringPlug made
	}
	if slices.Contains(leaks, "email") || slices.Contains(leaks, "phone") {
		return ClientCV{}, "shows their email or phone number: make one without contact details, or a blind one", nil
	}
	return ClientCV{Blind: made.Blind, Reference: "Candidate " + Reference(candidateID)}, "", nil
}
