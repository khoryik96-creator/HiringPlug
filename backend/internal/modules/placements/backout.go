package placements

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// BackOut marks an application's placement as backed out, in the caller's
// transaction: the candidate was moved out of placed (back to offer, or
// withdrawn) because they or the client pulled out. The sheet stays as it
// was; its fee no longer counts. One that already fell through or did not
// start is left as it is. No placement: nothing happens.
func (m *Module) BackOut(ctx context.Context, tx pgx.Tx, applicationID string) error {
	row, err := store.New(tx).BackOutPlacement(ctx, applicationID)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "placement.backed_out", EntityType: "placement", EntityID: row.ID,
		Changes: map[string]any{"status": map[string]string{"from": Active, "to": BackedOut}, "via": "moved out of placed"},
	})
}

// insertOrRevive records an application's placement: a new one, or its own
// again when that one was backed out of or did not start (the sheet becomes
// what is recorded now; what was invoiced stays). again reports the second.
func (s *Service) insertOrRevive(ctx context.Context, q *store.Queries, appID string, d Details) (row store.Placement, again bool, err error) {
	old, err := q.PlacementOfApplication(ctx, appID)
	if database.IsNoRows(err) {
		row, err = q.InsertPlacement(ctx, store.InsertPlacementParams{
			ApplicationID: appID, RecruiterID: d.RecruiterID, StartDate: d.StartDate, GuaranteeDays: int32(d.GuaranteeDays), //nolint:gosec // 0..365
			JobTitle: d.JobTitle, CompanyName: d.CompanyName, JobType: d.JobType, PoNumber: d.PONumber, EndDate: d.EndDate,
		})
		if database.IsUniqueViolation(err, onePerApplication) {
			return row, false, notAllowed("This application already has a placement: change that one.")
		}
		return row, false, err
	}
	if err != nil {
		return old, false, err
	}
	if Stood(old.Status) {
		return old, false, notAllowed("This application already has a placement: change that one.")
	}
	row, err = q.UpdatePlacement(ctx, store.UpdatePlacementParams{
		ID: old.ID, ExpectedVersion: old.Version,
		RecruiterID: d.RecruiterID, StartDate: d.StartDate, GuaranteeDays: int32(d.GuaranteeDays), //nolint:gosec // 0..365
		Status: Active, Replacement: NoReplacement,
		InvoiceStatus: old.InvoiceStatus, InvoiceNumber: old.InvoiceNumber, InvoicedOn: old.InvoicedOn, PaidOn: old.PaidOn,
		JobTitle: d.JobTitle, CompanyName: d.CompanyName, JobType: d.JobType, PoNumber: d.PONumber, EndDate: d.EndDate,
	})
	return row, true, err
}

// OnApplications reports which of some applications have a placement, in the
// caller's transaction (merging candidates never deletes those).
func (m *Module) OnApplications(ctx context.Context, tx pgx.Tx, applicationIDs []string) (map[string]bool, error) {
	ids, err := store.New(tx).ApplicationsWithPlacements(ctx, applicationIDs)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}
