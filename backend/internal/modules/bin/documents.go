package bin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) recordDocument(ctx context.Context, tx pgx.Tx, tenantID, id, name, actorID string) error {
	actor := "HyPlug"
	if actorID != "" {
		names, err := s.deps.Users.UserNames(ctx, tx, []string{actorID})
		if err != nil {
			return err
		}
		if names[actorID] != "" {
			actor = names[actorID]
		}
	}
	q := store.New(tx)
	entry, err := q.InsertDocumentBinEntry(ctx, store.InsertDocumentBinEntryParams{
		RecordID: id, Label: name, BinnedBy: actorID, BinnedName: actor, PurgeAfter: time.Now().Add(Keep),
	})
	if err != nil {
		return err
	}
	if _, err := jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindBinPurge, api.BinPurgeJob{EntryId: entry.ID}, entry.PurgeAfter); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "document.binned", EntityType: "document", EntityID: id})
}
