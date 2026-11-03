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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

// deliver handles webhook.deliver: it posts one delivery and records the
// outcome. A failure is tried again later (NextAttempt), with its reason
// recorded, until the last attempt fails it.
func (s *Service) deliver(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.WebhookDeliverJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	d, err := q.GetDeliveryToSend(ctx, payload.DeliveryId)
	if database.IsNoRows(err) {
		return nil // the webhook was removed
	}
	if err != nil || d.Status != "pending" {
		return err
	}
	if !d.Active {
		msg := "The webhook is turned off."
		return q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "failed", LastError: &msg})
	}
	now := time.Now()
	code, sendErr := s.post(ctx, d, now)
	var status *int32
	if code != 0 {
		c := int32(code) //nolint:gosec // an HTTP status
		status = &c
	}
	if sendErr == nil {
		return q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "delivered", ResponseStatus: status})
	}
	msg := sendErr.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	next, again := NextAttempt(int(d.Attempts)+1, now)
	if !again {
		return q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "failed", ResponseStatus: status, LastError: &msg})
	}
	if err := q.RecordAttempt(ctx, store.RecordAttemptParams{ID: d.ID, Status: "pending", ResponseStatus: status, LastError: &msg, NextAttemptAt: &next}); err != nil {
		return err
	}
	_, err = jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindWebhookDeliver, payload, next)
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
