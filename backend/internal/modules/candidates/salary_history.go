package candidates

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Salary snapshots are immutable and use compensation RLS, never general notes or audit payloads.
func saveCallSalary(ctx context.Context, tx pgx.Tx, p authz.Principal, call *ScreeningCall, current *Compensation, in *SalaryInput) error {
	if !p.Can(api.PermissionCandidatesCompensationRead) {
		return nil
	}
	recorded := current
	if in != nil && !call.SalaryRecorded {
		recorded = &Compensation{Current: in.Current, Expected: in.Expected, SalaryNotes: in.SalaryNotes.Value}
	}
	if recorded == nil {
		return nil
	}
	c := compensationToAPI(*recorded)
	snap := api.ScreeningSalarySnapshot{CurrentSalary: c.CurrentSalary, ExpectedSalary: c.ExpectedSalary, SalaryNotes: c.SalaryNotes}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := store.New(tx).InsertScreeningSalarySnapshot(ctx, store.InsertScreeningSalarySnapshotParams{CallID: call.ID, Snapshot: data}); err != nil {
		return err
	}
	call.SalarySnapshot = &snap
	return nil
}
func loadCallSalaries(ctx context.Context, tx pgx.Tx, calls []ScreeningCall) error {
	ids := make([]string, len(calls))
	byID := map[string]int{}
	for i, c := range calls {
		ids[i] = c.ID
		byID[c.ID] = i
	}
	rows, err := store.New(tx).ScreeningSalarySnapshots(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var snapshot api.ScreeningSalarySnapshot
		if err := json.Unmarshal(r.Snapshot, &snapshot); err != nil {
			return err
		}
		if i, ok := byID[r.CallID]; ok {
			calls[i].SalarySnapshot = &snapshot
		}
	}
	return nil
}
