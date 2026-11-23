package shortlists

// The client's page: no sign-in. The link's token is turned into its company
// (app_client_shortlist_tenant); everything after runs in that company's
// tenant scope as the app. The client sees only what the recruiter put on
// the link, and candidates who withdrew consent since drop off it.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/shortlists/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Page is what the client sees.
type Page struct {
	AgencyName, JobTitle, Status string
	RecruiterName, Message       *string
	ExpiresAt                    time.Time
	Candidates                   []PublicCandidate
}

// PublicCandidate is a candidate as the client sees them.
type PublicCandidate struct {
	ID, Label, CVFileName   string
	Blind                   bool
	Note, Decision, Comment *string
	AnsweredAt              *time.Time
}

func (s *Service) tenant(ctx context.Context, token string) (authz.Principal, string, string, error) {
	hash := HashToken(token)
	row, err := store.New(s.pool).ShortlistTenant(ctx, hash)
	if database.IsNoRows(err) {
		return authz.Principal{}, "", "", apperr.NotFound("shortlist link")
	}
	if err != nil {
		return authz.Principal{}, "", "", err
	}
	return authz.System(row.TenantID), row.TenantName, hash, nil
}

// page builds the client's page; a closed link shows no one.
func (s *Service) page(ctx context.Context, tx pgx.Tx, agency string, row store.ClientShortlist) (Page, error) {
	job, err := s.deps.Jobs.Job(ctx, tx, row.JobID)
	if err != nil {
		return Page{}, err
	}
	out := Page{AgencyName: agency, JobTitle: job.Title, Status: Status(row.ExpiresAt, row.RevokedAt, s.now()), Message: row.Message, ExpiresAt: row.ExpiresAt}
	names, err := s.deps.Users.UserNames(ctx, tx, []string{row.CreatedBy})
	if err != nil {
		return Page{}, err
	}
	if n, ok := names[row.CreatedBy]; ok {
		out.RecruiterName = &n
	}
	if out.Status != Open {
		out.Message = nil
		return out, nil
	}
	items, err := store.New(tx).ShortlistCandidates(ctx, []string{row.ID})
	if err != nil {
		return Page{}, err
	}
	for _, it := range items {
		no, err := s.deps.Consent.DoNotContact(ctx, tx, it.CandidateID)
		if err != nil {
			return Page{}, err
		}
		if no {
			continue
		}
		stored, err := s.deps.CVs.FileName(ctx, tx, it.DocumentID) // the PDF, or the Word file while it has none
		if err != nil {
			return Page{}, err
		}
		out.Candidates = append(out.Candidates, PublicCandidate{
			ID: it.ID, Label: it.Label, CVFileName: CVFileName(it.Label, stored), Blind: it.Blind, Note: it.Note,
			Decision: it.Decision, Comment: it.Comment, AnsweredAt: it.AnsweredAt,
		})
	}
	return out, nil
}

// PublicPage returns the client's page; the first visit to an open link is
// recorded on the job's activity.
func (s *Service) PublicPage(ctx context.Context, token string) (Page, error) {
	p, agency, hash, err := s.tenant(ctx, token)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.ShortlistByToken(ctx, hash)
		if err != nil {
			return err
		}
		if Status(row.ExpiresAt, row.RevokedAt, s.now()) == Open {
			first, err := q.MarkShortlistOpened(ctx, row.ID)
			if err != nil {
				return err
			}
			if first {
				if err := audit.Record(ctx, tx, audit.Entry{
					Action: "client_shortlist.opened", EntityType: "job", EntityID: row.JobID, Changes: map[string]any{"shortlistId": row.ID},
				}); err != nil {
					return err
				}
			}
		}
		out, err = s.page(ctx, tx, agency, row)
		return err
	})
	return out, err
}

// open finds an open link's candidate for the client, locked; not found
// when they are not on it (or withdrew consent).
func (s *Service) open(ctx context.Context, tx pgx.Tx, hash, itemID string) (store.ClientShortlist, store.ClientShortlistCandidate, error) {
	q := store.New(tx)
	row, err := q.LockShortlistByToken(ctx, hash)
	if err != nil {
		return row, store.ClientShortlistCandidate{}, err
	}
	if Status(row.ExpiresAt, row.RevokedAt, s.now()) != Open {
		return row, store.ClientShortlistCandidate{}, apperr.New(api.ErrorCodeSHORTLISTCLOSED, "This link no longer works: ask the recruiter for a new one.")
	}
	it, err := q.LockShortlistCandidate(ctx, store.LockShortlistCandidateParams{ID: itemID, ShortlistID: row.ID})
	if database.IsNoRows(err) {
		return row, it, apperr.NotFound("candidate")
	}
	if err != nil {
		return row, it, err
	}
	no, err := s.deps.Consent.DoNotContact(ctx, tx, it.CandidateID)
	if err == nil && no {
		err = apperr.NotFound("candidate")
	}
	return row, it, err
}

// Answer records the client's answer about a candidate (once) and moves the
// application by it.
func (s *Service) Answer(ctx context.Context, token, itemID, decision string, comment *string, client string) (Page, error) {
	comment, errs := NormalizeAnswer(decision, comment)
	if err := errs.Err(); err != nil {
		return Page{}, err
	}
	if !s.limiter.Allow(client) {
		return Page{}, tooMany()
	}
	p, agency, hash, err := s.tenant(ctx, token)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, it, err := s.open(ctx, tx, hash, itemID)
		if err != nil {
			return err
		}
		if it.Decision != nil {
			return apperr.New(api.ErrorCodeSHORTLISTALREADYANSWERED, "You have already answered about this candidate. To change it, tell the recruiter.")
		}
		if _, err := store.New(tx).AnswerShortlistCandidate(ctx, store.AnswerShortlistCandidateParams{ID: it.ID, Decision: &decision, Comment: comment}); err != nil {
			return err
		}
		var stage string
		if decision == Approved {
			stage, err = s.deps.Applications.Approved(ctx, tx, it.ApplicationID)
		} else {
			stage, err = s.deps.Applications.Rejected(ctx, tx, it.ApplicationID, val(comment))
		}
		if err != nil {
			return err
		}
		job, err := s.deps.Jobs.Job(ctx, tx, row.JobID)
		if err != nil {
			return err
		}
		names, err := s.deps.Candidates.Names(ctx, tx, []string{it.CandidateID})
		if err != nil {
			return err
		}
		about := map[string]any{"shortlistId": row.ID, "decision": decision, "comment": comment, "stage": stage}
		onCandidate := map[string]any{"jobId": row.JobID, "jobTitle": job.Title}
		onJob := map[string]any{"candidateId": it.CandidateID, "candidateName": names[it.CandidateID]}
		for k, v := range about {
			onCandidate[k], onJob[k] = v, v
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "client_shortlist.answered", EntityType: "candidate", EntityID: it.CandidateID, Changes: onCandidate}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "client_shortlist.answered", EntityType: "job", EntityID: row.JobID, Changes: onJob}); err != nil {
			return err
		}
		out, err = s.page(ctx, tx, agency, row)
		return err
	})
	return out, err
}

// CV returns a candidate's CV for the client, named by the label.
func (s *Service) CV(ctx context.Context, token, itemID, client string) (CVFile, error) {
	if !s.limiter.Allow(client) {
		return CVFile{}, tooMany()
	}
	p, _, hash, err := s.tenant(ctx, token)
	if err != nil {
		return CVFile{}, err
	}
	var out CVFile
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		_, it, err := s.open(ctx, tx, hash, itemID)
		if err != nil {
			return err
		}
		f, err := s.deps.CVs.File(ctx, tx, it.DocumentID)
		if err != nil {
			return err
		}
		out = CVFile{FileName: CVFileName(it.Label, f.FileName), Data: f.Data}
		return nil
	})
	return out, err
}

func tooMany() error {
	return apperr.New(api.ErrorCodeSHORTLISTTOOMANYTRIES, "Too many tries from your address; try again in an hour.")
}

func val(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
