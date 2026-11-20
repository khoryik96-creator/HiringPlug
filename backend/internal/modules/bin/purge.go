package bin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// purge handles bin.purge: once a record's 30 days in the Bin are over, it is
// deleted for good with what belongs to it. A restored record is left alone.
// A company still referred to by something else in the Bin (a contact binned
// on its own, deleted later) waits a day and tries again. Stored files are
// removed last (the rows referring to them are gone by then).
func (s *Service) purge(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.BinPurgeJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	if err := q.ShowBinned(ctx); err != nil {
		return err
	}
	e, err := q.GetBinEntry(ctx, payload.EntryId)
	if database.IsNoRows(err) {
		return nil // restored
	}
	if err != nil {
		return err
	}
	if time.Now().Before(e.PurgeAfter) {
		return jobqueue.Later(e.PurgeAfter)
	}
	sp, err := tx.Begin(ctx) // a savepoint: a record still referred to rolls back to here
	if err != nil {
		return err
	}
	keys, err := s.purgeRecords(ctx, sp, e)
	var fk *pgconn.PgError
	if errors.As(err, &fk) && fk.Code == "23503" {
		_ = sp.Rollback(ctx)
		return jobqueue.Later(time.Now().Add(24 * time.Hour))
	}
	if err != nil {
		return err
	}
	if err := sp.Commit(ctx); err != nil {
		return err
	}
	if _, err := q.DeleteBinEntry(ctx, e.ID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: e.Kind + ".deleted", EntityType: e.Kind, EntityID: e.RecordID,
		Changes: addLabel(map[string]any{"binnedBy": e.BinnedName, "binnedAt": e.BinnedAt}, e.Kind, e.Label)}); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	_, err = jobqueue.Enqueue(ctx, tx, job.TenantID, api.JobKindBinRemoveFiles, api.BinRemoveFilesJob{Keys: keys})
	return err
}

func (s *Service) removeFiles(_ context.Context, _ pgx.Tx, job jobqueue.Job) error {
	var payload api.BinRemoveFilesJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	for _, key := range payload.Keys {
		if !strings.HasPrefix(key, job.TenantID+"/") {
			return errors.New("purged file belongs to a different tenant")
		}
		if err := s.deps.DeleteFile(key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) purgeRecords(ctx context.Context, tx pgx.Tx, e store.BinEntry) ([]string, error) {
	d := s.deps
	var keys []string
	keep := func(removed []string, err error) error {
		keys = append(keys, removed...)
		return err
	}
	switch e.Kind {
	case "candidate":
		apps, err := d.Pipeline.ApplicationsOf(ctx, tx, []string{e.RecordID}, nil)
		if err == nil {
			err = d.Pipeline.Purge(ctx, tx, appIDs(apps))
		}
		if err == nil {
			err = d.Files.DeleteNotes(ctx, tx, "candidate", []string{e.RecordID})
		}
		if err == nil {
			err = keep(d.Files.PurgeCandidate(ctx, tx, e.RecordID))
		}
		if err == nil {
			err = d.Candidates.Purge(ctx, tx, e.RecordID)
		}
		return keys, err
	case "contact":
		return keys, s.purgeContacts(ctx, tx, []string{e.RecordID})
	}
	went := decodeWentWith(e.WentWith)
	err := s.purgeContacts(ctx, tx, went.Contacts)
	if err == nil && len(went.Jobs) > 0 {
		var apps []Application
		if apps, err = d.Pipeline.ApplicationsOf(ctx, tx, nil, went.Jobs); err == nil {
			err = d.Pipeline.Purge(ctx, tx, appIDs(apps))
		}
		if err == nil {
			err = d.Files.DeleteNotes(ctx, tx, "job", went.Jobs)
		}
		if err == nil {
			err = keep(d.Jobs.Purge(ctx, tx, went.Jobs))
		}
	}
	if err == nil {
		err = d.Files.DeleteNotes(ctx, tx, "company", []string{e.RecordID})
	}
	if err == nil {
		err = keep(d.Companies.Purge(ctx, tx, e.RecordID))
	}
	return keys, err
}

func (s *Service) purgeContacts(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.deps.Jobs.ClearContacts(ctx, tx, ids); err != nil {
		return err
	}
	if err := s.deps.Files.DeleteNotes(ctx, tx, "contact", ids); err != nil {
		return err
	}
	return s.deps.Contacts.Purge(ctx, tx, ids)
}
