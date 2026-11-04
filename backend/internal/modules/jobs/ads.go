package jobs

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Ad is a job ad.
type Ad struct {
	ID          string
	JobID       string
	Version     int
	Details     AdDetails
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListAds returns a job's ads, newest first.
func (s *Service) ListAds(ctx context.Context, p authz.Principal, jobID string) ([]Ad, error) {
	var ads []Ad
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := getRow(ctx, q, jobID); err != nil {
			return err
		}
		rows, err := q.ListJobAds(ctx, jobID)
		if err != nil {
			return err
		}
		ads = make([]Ad, len(rows))
		for i, r := range rows {
			ads[i] = adFromRow(r)
		}
		return nil
	})
	return ads, err
}

// CreateAd adds a draft ad for a job.
func (s *Service) CreateAd(ctx context.Context, p authz.Principal, jobID string, d AdDetails) (Ad, error) {
	d.Status = "draft"
	d, errs := NormalizeAd(d)
	if err := errs.Err(); err != nil {
		return Ad{}, err
	}
	var ad Ad
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := getRow(ctx, q, jobID); err != nil {
			return err
		}
		row, err := q.InsertJobAd(ctx, store.InsertJobAdParams{
			JobOrderID: jobID, Title: d.Title, Location: d.Location, WorkType: d.WorkType,
			WorkArrangement: d.WorkArrangement, SalaryText: d.SalaryText, Description: d.Description, ClosesOn: toDate(d.ClosesOn),
		})
		if err != nil {
			return err
		}
		ad = adFromRow(row)
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job_ad.created", EntityType: "job_ad", EntityID: ad.ID, Changes: map[string]any{"jobId": jobID, "title": d.Title},
		})
	})
	return ad, err
}

// GetAd returns one ad.
func (s *Service) GetAd(ctx context.Context, p authz.Principal, id string) (Ad, error) {
	var ad Ad
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getAdRow(ctx, store.New(tx), id)
		ad = adFromRow(row)
		return err
	})
	return ad, err
}

// UpdateAd changes an ad and its status (publish, unpublish, close).
func (s *Service) UpdateAd(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch AdPatch) (Ad, error) {
	var ad Ad
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getAdRow(ctx, q, id)
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("job ad").WithDetail("currentVersion", row.Version)
		}
		current := adFromRow(row)
		after, errs := NormalizeAd(patch.Apply(current.Details))
		if err := errs.Err(); err != nil {
			return err
		}
		changes := audit.Diff(adFields(current.Details), adFields(after))
		if len(changes) == 0 {
			ad = current
			return nil
		}
		publishedAt := current.PublishedAt
		if after.Status == "published" {
			job, err := getRow(ctx, q, row.JobOrderID)
			if err != nil {
				return err
			}
			becoming := current.Details.Status != "published"
			if problem := PublishProblem(after, job.Status, today(), becoming); problem != "" {
				return apperr.New(api.ErrorCodeJOBADNOTPUBLISHABLE, problem)
			}
			if becoming {
				now := time.Now()
				publishedAt = &now
			}
		}
		updated, err := q.UpdateJobAd(ctx, store.UpdateJobAdParams{
			ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small positive integers
			Title: after.Title, Location: after.Location, WorkType: after.WorkType, WorkArrangement: after.WorkArrangement,
			SalaryText: after.SalaryText, Description: after.Description, ClosesOn: toDate(after.ClosesOn),
			Status: after.Status, PublishedAt: publishedAt,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("job ad")
		}
		if err != nil {
			return err
		}
		ad = adFromRow(updated)
		return audit.Record(ctx, tx, audit.Entry{Action: "job_ad.updated", EntityType: "job_ad", EntityID: id, Changes: changes})
	})
	return ad, err
}

// closeAdsWithJob takes a job's published ads down when the job is filled or
// closed, in the job's own transaction.
func closeAdsWithJob(ctx context.Context, tx pgx.Tx, jobID, status string) error {
	if status != "filled" && status != "closed" {
		return nil
	}
	n, err := store.New(tx).CloseJobAds(ctx, jobID)
	if err != nil || n == 0 {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "job_ad.closed_with_job", EntityType: "job", EntityID: jobID, Changes: map[string]any{"ads": n, "jobStatus": status},
	})
}

func getAdRow(ctx context.Context, q *store.Queries, id string) (store.JobAd, error) {
	row, err := q.GetJobAd(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("job ad")
	}
	return row, err
}

func adFromRow(r store.JobAd) Ad {
	d := AdDetails{
		Title: r.Title, Location: r.Location, WorkType: r.WorkType, WorkArrangement: r.WorkArrangement,
		SalaryText: r.SalaryText, Description: r.Description, Status: r.Status,
	}
	if r.ClosesOn.Valid {
		t := r.ClosesOn.Time
		d.ClosesOn = &t
	}
	return Ad{ID: r.ID, JobID: r.JobOrderID, Version: int(r.Version), Details: d, PublishedAt: r.PublishedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// adFields are the audited fields; the text is long, so only whether it changed.
func adFields(d AdDetails) map[string]any {
	closes := ""
	if d.ClosesOn != nil {
		closes = d.ClosesOn.Format(time.DateOnly)
	}
	text := 0
	if d.Description != nil {
		text = len(*d.Description)
	}
	return map[string]any{
		"title": d.Title, "location": d.Location, "workType": d.WorkType, "workArrangement": d.WorkArrangement,
		"salaryText": d.SalaryText, "closesOn": closes, "status": d.Status, "textLength": text,
	}
}

func toDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func today() time.Time {
	y, m, d := time.Now().UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// PublishedAds returns the ads on the careers page: published and not past
// their closing date, newest first, in the caller's transaction.
func (m *Module) PublishedAds(ctx context.Context, tx pgx.Tx) ([]Ad, error) {
	rows, err := store.New(tx).ListPublishedAds(ctx)
	if err != nil {
		return nil, err
	}
	ads := make([]Ad, len(rows))
	for i, r := range rows {
		ads[i] = adFromRow(r)
	}
	return ads, nil
}

// BestAd returns the ad that best describes a job, for other modules (CV vs
// job matching): its newest published ad with text, else its newest ad with
// text; nil when it has none. In the caller's transaction.
func (m *Module) BestAd(ctx context.Context, tx pgx.Tx, jobID string) (*Ad, error) {
	rows, err := store.New(tx).ListJobAds(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var best *Ad
	for _, r := range rows { // newest first
		if r.Description == nil || strings.TrimSpace(*r.Description) == "" {
			continue
		}
		if r.Status == "published" {
			ad := adFromRow(r)
			return &ad, nil
		}
		if best == nil {
			ad := adFromRow(r)
			best = &ad
		}
	}
	return best, nil
}

// PublishedAd returns one ad on the careers page; anything else (a draft, a
// closed or expired ad) is not found.
func (m *Module) PublishedAd(ctx context.Context, tx pgx.Tx, id string) (Ad, error) {
	row, err := store.New(tx).GetPublishedAd(ctx, id)
	if database.IsNoRows(err) {
		return Ad{}, apperr.NotFound("job ad")
	}
	return adFromRow(row), err
}
