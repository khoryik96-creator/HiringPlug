package emails

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) deliverOutside(ctx context.Context, job jobqueue.Job) error {
	var payload api.EmailSendJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	p := authz.System(job.TenantID)
	var claimed int64
	if err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		claimed, err = store.New(tx).BeginEmailDelivery(ctx, payload.EmailId)
		return err
	}); err != nil {
		return err
	}
	var mayHaveDelivered bool
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		return s.deliver(ctx, tx, job, payload, claimed == 1, &mayHaveDelivered)
	})
	if err == nil || claimed != 1 || mayHaveDelivered {
		return err
	}
	// A live worker positively knows it failed before delivery. A worker that
	// died or lost a provider receipt cannot make that determination.
	release := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		return store.New(tx).ReleaseEmailDelivery(ctx, payload.EmailId)
	})
	return errors.Join(err, release)
}
