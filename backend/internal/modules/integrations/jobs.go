package integrations

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

// deliver handles webhook.deliver, with no queue transaction open while the
// receiver answers (it may take its 10 seconds): it reads the delivery, posts
// it, and records the outcome in a transaction of its own. A failure is tried
// again later (NextAttempt), with its reason recorded, until the last
// attempt fails it. A delivery recorded meanwhile (this job ran twice) is
// left as it is.
func (s *Service) deliver(ctx context.Context, job jobqueue.Job) error {
	var payload api.WebhookDeliverJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	var d store.GetDeliveryToSendRow
	send := false
	err := database.InTx(ctx, s.pool, authz.System(job.TenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		d, err = q.GetDeliveryToSend(ctx, payload.DeliveryId)
		if database.IsNoRows(err) {
			return nil // the webhook was removed
		}
		if err != nil || d.Status != "pending" {
			return err
		}
		if !d.Active {
			msg := "The webhook is turned off."
			return q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "failed", LastError: &msg, Attempts: d.Attempts})
		}
		secret, restore, err := s.sealer.Open(job.TenantID, d.Secret)
		if err != nil {
			msg := err.Error()
			return q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "failed", LastError: &msg, Attempts: d.Attempts})
		}
		if restore { // saved in the clear, or sealed by an earlier version's key: sealed (again) now
			if stored, err := s.sealer.Seal(job.TenantID, secret); err != nil {
				return err
			} else if err := q.SealWebhookSecret(ctx, store.SealWebhookSecretParams{Sealed: stored, Plain: d.Secret}); err != nil {
				return err
			}
		}
		d.Secret, send = secret, true
		return nil
	})
	if err != nil || !send {
		return err
	}
	now := time.Now()
	code, sendErr := s.post(ctx, d, now)
	return database.InTx(ctx, s.pool, authz.System(job.TenantID), func(tx pgx.Tx) error {
		return s.record(ctx, tx, job, payload, d, code, sendErr, now)
	})
}

// record saves how a delivery went, and queues its next try when it failed.
func (s *Service) record(ctx context.Context, tx pgx.Tx, job jobqueue.Job, payload api.WebhookDeliverJob, d store.GetDeliveryToSendRow, code int, sendErr error, now time.Time) error {
	q := store.New(tx)
	var status *int32
	if code != 0 {
		c := int32(code) //nolint:gosec // an HTTP status
		status = &c
	}
	at := store.RecordAttemptParams{ID: d.ID, Status: "delivered", ResponseStatus: status, Attempts: d.Attempts}
	if sendErr == nil {
		return q.RecordAttempt(ctx, at)
	}
	msg := sendErr.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	at.LastError = &msg
	next, again := NextAttempt(int(d.Attempts)+1, now)
	if !again {
		at.Status = "failed"
		return q.RecordAttempt(ctx, at)
	}
	at.Status, at.NextAttemptAt = "pending", &next
	if err := q.RecordAttempt(ctx, at); err != nil {
		return err
	}
	_, err := jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindWebhookDeliver, payload, next)
	return err
}

// DeadJobHandlers fail a webhook delivery whose job ended without posting
// it (cancelled after a restore, or stopped for good): pending for ever
// otherwise.
func (m *Module) DeadJobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindWebhookDeliver: m.service.deliveryDied}
}

func (s *Service) deliveryDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.WebhookDeliverJob
	if err := job.Decode(&payload); err != nil {
		return nil //nolint:nilerr // an unreadable job names no delivery
	}
	why := "Not sent: its delivery stopped before it was posted."
	if jobqueue.WasCancelledAfterRestore(job) {
		why = "Not sent: cancelled after the server was restored (it was waiting in the backup)."
	}
	_, err := store.New(tx).FailPendingDelivery(ctx, store.FailPendingDeliveryParams{ID: payload.DeliveryId, LastError: &why})
	return err
}

// post sends a delivery, signed, and returns the HTTP status (0 when there
// was none) and an error unless the receiver answered 2xx.
func (s *Service) post(ctx context.Context, d store.GetDeliveryToSendRow, now time.Time) (int, error) {
	client := safehttp.Client(safehttp.Options{AllowPrivate: s.allowPrivate, Timeout: 10 * time.Second})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.Url, bytes.NewReader(d.Payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HiringPlug-Webhooks/1")
	req.Header.Set("X-HiringPlug-Event", d.Event)
	req.Header.Set("X-HiringPlug-Delivery", d.ID)
	req.Header.Set("X-HiringPlug-Signature", Signature(d.Secret, now, d.Payload))
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode/100 != 2 {
		if res.StatusCode/100 == 3 {
			return res.StatusCode, safehttp.ErrRedirect
		}
		return res.StatusCode, fmt.Errorf("the receiver answered HTTP %d", res.StatusCode)
	}
	return res.StatusCode, nil
}
