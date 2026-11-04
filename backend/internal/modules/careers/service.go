package careers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// Service holds the careers use cases.
type Service struct {
	pool      *pgxpool.Pool
	deps      Deps
	limiter   *ratelimit.Limiter
	publicURL string
}

// Feed returns the careers page's open ads as an XML job feed.
func (s *Service) Feed(ctx context.Context, slug string) ([]byte, error) {
	name, ads, err := s.Page(ctx, slug)
	if err != nil {
		return nil, err
	}
	return Feed(name, slug, s.publicURL, ads, time.Now())
}

// tenant turns a page address into the company's tenant scope.
func (s *Service) tenant(ctx context.Context, slug string) (authz.Principal, string, error) {
	tenantID, name, ok, err := s.deps.Tenants.CareersTenant(ctx, slug)
	if err != nil {
		return authz.Principal{}, "", err
	}
	if !ok {
		return authz.Principal{}, "", apperr.NotFound("careers page")
	}
	return authz.System(tenantID), name, nil
}

// Page returns the company's name and its published ads.
func (s *Service) Page(ctx context.Context, slug string) (string, []Ad, error) {
	p, name, err := s.tenant(ctx, slug)
	if err != nil {
		return "", nil, err
	}
	var ads []Ad
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ads, err = s.deps.Ads.PublishedAds(ctx, tx)
		return err
	})
	return name, ads, err
}

// Ad returns one published ad.
func (s *Service) Ad(ctx context.Context, slug, adID string) (Ad, error) {
	p, _, err := s.tenant(ctx, slug)
	if err != nil {
		return Ad{}, err
	}
	var ad Ad
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ad, err = s.deps.Ads.PublishedAd(ctx, tx, adID)
		return err
	})
	return ad, err
}

// Apply adds the applicant (or finds them by email), their CV and their
// application to the ad's job, in one transaction.
func (s *Service) Apply(ctx context.Context, slug, adID, clientAddr string, f Form, fileName string, cv []byte) error {
	if f.IsRobot() {
		logx.From(ctx).Info("careers form trap filled; ignored", "ad_id", adID)
		return nil
	}
	applicant, errs := f.Validate()
	if err := errs.Err(); err != nil {
		return err
	}
	if !s.limiter.Allow(clientAddr) {
		return apperr.New(api.ErrorCodeCAREERSTOOMANYAPPLICATIONS, "Too many applications from your connection. Please try again later.")
	}
	p, _, err := s.tenant(ctx, slug)
	if err != nil {
		return err
	}
	undo := func() {}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ad, err := s.deps.Ads.PublishedAd(ctx, tx, adID)
		if err != nil {
			return err
		}
		candidateID, created, err := s.deps.Candidates.FindOrCreateApplicant(ctx, tx, applicant)
		if err != nil {
			return err
		}
		documentID, removeFile, err := s.deps.CVs.AddFile(ctx, tx, p.TenantID, candidateID, fileName, cv)
		undo = removeFile
		if err != nil {
			return err
		}
		added, err := s.deps.Pipeline.AddApplied(ctx, tx, p.TenantID, ad.JobID, candidateID, applicant.Channel)
		if err != nil {
			return err
		}
		if err := s.deps.Consent.RecordFormConsent(ctx, tx, candidateID); err != nil {
			return err
		}
		// Counts and ids only: no names or contact details in the audit log.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "careers.applied", EntityType: "candidate", EntityID: candidateID,
			Changes: map[string]any{
				"adId": adID, "jobId": ad.JobID, "documentId": documentID, "newCandidate": created,
				"addedToJob": added, "channel": applicant.Channel, "consent": true,
			},
		})
	})
	if err != nil {
		undo() // the transaction rolled back, so nothing refers to the stored CV
	}
	return err
}
