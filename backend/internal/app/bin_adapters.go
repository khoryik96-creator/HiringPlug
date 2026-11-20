package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
)

// binDeps gives the Bin the modules that own the records it hides, restores and deletes.
func binDeps(m *Modules, deleteFile func(string) error) bin.Deps {
	return bin.Deps{
		DeleteFile: deleteFile,
		Candidates: m.Candidates, Contacts: m.Contacts, Companies: m.Companies, Jobs: m.Jobs,
		Pipeline: binPipeline{m.Pipeline, m.Placements, m.Assistant, m},
		Files:    binFiles{m.Documents, m.Notes, m.Assistant},
		Taken: func(err error) bool {
			return errors.Is(err, candidates.ErrEmailTaken) || errors.Is(err, contacts.ErrEmailTaken) || errors.Is(err, companies.ErrNameTaken)
		},
	}
}

type binPipeline struct {
	pipeline   *pipeline.Module
	placements *placements.Module
	assistant  *assistant.Module
	modules    *Modules // interviews and calendars, wired after the Bin
}

func (a binPipeline) ApplicationsOf(ctx context.Context, tx pgx.Tx, candidateIDs, jobIDs []string) ([]bin.Application, error) {
	got, err := a.pipeline.ApplicationsOf(ctx, tx, candidateIDs, jobIDs)
	out := make([]bin.Application, len(got))
	for i, g := range got {
		out[i] = bin.Application{ID: g.ID, CandidateID: g.CandidateID, JobID: g.JobID}
	}
	return out, err
}

func (a binPipeline) Placed(ctx context.Context, tx pgx.Tx, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	on, err := a.placements.OnApplications(ctx, tx, ids)
	return len(on), err
}

func (a binPipeline) SetBinned(ctx context.Context, tx pgx.Tx, ids []string, binned bool) error {
	if len(ids) == 0 {
		return nil
	}
	return a.pipeline.SetBinned(ctx, tx, ids, binned)
}

func (a binPipeline) Purge(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := a.assistant.Forget(ctx, tx, "application", ids); err != nil {
		return err
	}
	// Their interviews go with them: Outlook invitations are cancelled, and
	// the candidate's address no longer kept for them.
	interviewIDs, err := a.modules.Interviews.IDsOnApplications(ctx, tx, ids)
	if err != nil {
		return err
	}
	if err := a.modules.Calendars.InterviewsDeleted(ctx, tx, interviewIDs); err != nil {
		return err
	}
	return a.pipeline.Purge(ctx, tx, ids)
}

type binFiles struct {
	documents *documents.Module
	notes     *notes.Module
	assistant *assistant.Module
}

func (a binFiles) PurgeCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]string, error) {
	return a.documents.PurgeCandidate(ctx, tx, candidateID)
}

func (a binFiles) DeleteNotes(ctx context.Context, tx pgx.Tx, subjectType string, ids []string) error {
	if subjectType == "candidate" || subjectType == "job" { // what the AI said about them goes too
		if err := a.assistant.Forget(ctx, tx, subjectType, ids); err != nil {
			return err
		}
	}
	return a.notes.DeleteFor(ctx, tx, subjectType, ids)
}
