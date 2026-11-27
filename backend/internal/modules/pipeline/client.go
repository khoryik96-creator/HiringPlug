package pipeline

// The client's answer on a shortlist link moves the application: approved
// goes on to interview, rejected closes it (client rejected).

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ClientRejected is the reason an application is closed with when the client says no.
const ClientRejected = "client_rejected"

const viaShortlist = "client shortlist link"

// ClientApprovedStep is where an approved application goes: the job's first
// interview stage (nil with the standard stages), or ok false when the job
// has no interview stage.
func ClientApprovedStep(steps []Step, custom bool) (step *string, ok bool) {
	for _, st := range steps {
		if st.Stage == Interview {
			if !custom {
				return nil, true
			}
			key := st.Key
			return &key, true
		}
	}
	return nil, false
}

// ClientApproved moves an application the client approved to interview, in
// the caller's transaction, when it is at an earlier stage (applied,
// screening, submitted). It returns the stage it is at afterwards; an
// application that is gone returns "" and no error.
func (m *Module) ClientApproved(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	q := store.New(tx)
	row, err := q.GetApplication(ctx, id)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if row.Stage != Applied && row.Stage != Screening && row.Stage != Submitted {
		return row.Stage, nil
	}
	js, err := m.service.jobStages(ctx, tx, row.JobOrderID)
	if err != nil {
		return "", err
	}
	step, ok := ClientApprovedStep(js.Steps, js.Source != FromStandard)
	if !ok {
		return row.Stage, nil
	}
	return Interview, m.clientMove(ctx, tx, row, Interview, step, nil, nil)
}

// ClientRejectedWith closes an application the client rejected, in the
// caller's transaction: rejected, reason client rejected, their comment as
// the note. A placed or closed application stays as it is. It returns the
// stage it is at afterwards; an application that is gone returns "".
func (m *Module) ClientRejectedWith(ctx context.Context, tx pgx.Tx, id, comment string) (string, error) {
	q := store.New(tx)
	row, err := q.GetApplication(ctx, id)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if row.Stage == Placed || IsClosed(row.Stage) {
		return row.Stage, nil
	}
	reason := ClientRejected
	var note *string
	if c := strings.TrimSpace(comment); c != "" {
		if r := []rune(c); len(r) > 1000 {
			c = string(r[:1000])
		}
		note = &c
	}
	return Rejected, m.clientMove(ctx, tx, row, Rejected, nil, &reason, note)
}

func (m *Module) clientMove(ctx context.Context, tx pgx.Tx, row store.Application, to string, step, reason, note *string) error {
	category, err := m.service.appCategory(ctx, tx, row.JobOrderID, to, step)
	if err != nil {
		return err
	}
	moved, err := store.New(tx).MoveApplication(ctx, store.MoveApplicationParams{
		ID: row.ID, ExpectedVersion: row.Version, Stage: to, Step: step, Category: category, Reason: reason, ReasonNote: note,
	})
	if database.IsNoRows(err) {
		return apperr.StaleVersion("application")
	}
	if err != nil {
		return err
	}
	changes := map[string]any{"stage": map[string]string{"from": row.Stage, "to": moved.Stage}, "via": viaShortlist}
	if reason != nil {
		changes["reason"] = *reason
	}
	if note != nil {
		changes["reasonNote"] = *note
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "application.moved", EntityType: "application", EntityID: row.ID, Changes: changes})
}
