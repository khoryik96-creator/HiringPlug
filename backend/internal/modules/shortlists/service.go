package shortlists

// Use cases for signed-in users: making a link, the job's links with the
// client's answers, and revoking one.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/shortlists/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// Service holds the shortlists use cases.
type Service struct {
	pool      *pgxpool.Pool
	deps      Deps
	publicURL string
	now       func() time.Time
	limiter   *ratelimit.Limiter
}

// URL is the address of a link.
func (s *Service) URL(token string) string { return s.publicURL + "/shortlist/" + token }

// Create makes a link to some of a job's candidates; it returns the link and
// its address (shown only now).
func (s *Service) Create(ctx context.Context, p authz.Principal, jobID string, in Input) (Shortlist, string, error) {
	in, days, errs := NormalizeInput(in)
	if err := errs.Err(); err != nil {
		return Shortlist{}, "", err
	}
	if in.EmailHiringContact && !p.Can(api.PermissionEmailsSend) {
		return Shortlist{}, "", apperr.New(api.ErrorCodeFORBIDDEN, "Emailing the link needs permission to send emails: copy it instead.")
	}
	token, hash, err := NewToken()
	if err != nil {
		return Shortlist{}, "", err
	}
	var out Shortlist
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		job, err := s.deps.Jobs.Job(ctx, tx, jobID)
		if err != nil {
			return err
		}
		picked, err := s.checkPicks(ctx, tx, p, jobID, in.Picks)
		if err != nil {
			return err
		}
		var emailTo *string
		if in.EmailHiringContact {
			if job.ContactEmail == nil || *job.ContactEmail == "" {
				return apperr.Validation(apperr.FieldErrors{{Field: "emailHiringContact", Message: "the job has no hiring contact with an email address: copy the link instead"}})
			}
			emailTo = job.ContactEmail
		}
		expires := s.now().Add(time.Duration(days) * 24 * time.Hour)
		q := store.New(tx)
		row, err := q.InsertShortlist(ctx, store.InsertShortlistParams{
			TokenHash: hash, JobID: jobID, Message: in.Message, ExpiresAt: expires, EmailedTo: emailTo,
		})
		if err != nil {
			return err
		}
		for i, c := range picked {
			if _, err := q.InsertShortlistCandidate(ctx, store.InsertShortlistCandidateParams{
				ShortlistID: row.ID, Position: int16(i + 1), //nolint:gosec // at most 20
				ApplicationID: c.ApplicationID, CandidateID: c.CandidateID, DocumentID: c.DocumentID, Blind: c.Blind, Label: c.Label, Note: c.Note,
			}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "client_shortlist.shared", EntityType: "candidate", EntityID: c.CandidateID,
				Changes: map[string]any{"shortlistId": row.ID, "jobId": jobID, "jobTitle": job.Title, "documentId": c.DocumentID, "blind": c.Blind},
			}); err != nil {
				return err
			}
		}
		if emailTo != nil {
			agency, err := s.deps.Users.TenantName(ctx, tx, p.TenantID)
			if err != nil {
				return err
			}
			subject, body := LinkEmail(job.ContactName, job.Title, s.URL(token), len(picked), in.Message, expires, p.Name, agency)
			if err := s.deps.Mailer.Send(ctx, tx, p.TenantID, Mail{To: *emailTo, Subject: subject, Body: body, From: p.UserID, FromName: p.Name}); err != nil {
				return err
			}
		}
		changes := map[string]any{"shortlistId": row.ID, "candidates": len(picked), "expiresAt": expires.UTC()}
		if emailTo != nil {
			changes["emailedTo"] = *emailTo
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "client_shortlist.created", EntityType: "job", EntityID: jobID, Changes: changes}); err != nil {
			return err
		}
		views, err := s.views(ctx, tx, []store.ClientShortlist{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	if err != nil {
		return Shortlist{}, "", err
	}
	return out, s.URL(token), nil
}

// checkPicks checks each candidate: on this job and open, willing to be
// contacted, with a CV that may go to the client. It returns them as stored.
func (s *Service) checkPicks(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string, picks []Pick) ([]Candidate, error) {
	ids := make([]string, len(picks))
	for i, pk := range picks {
		ids[i] = pk.ApplicationID
	}
	apps, err := s.deps.Applications.Briefs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, len(picks))
	for i, pk := range picks {
		app, ok := apps[pk.ApplicationID]
		field := fmt.Sprintf("candidates.%d.applicationId", i)
		switch {
		case !ok || app.JobID != jobID:
			return nil, apperr.Validation(apperr.FieldErrors{{Field: field, Message: "is not on this job"}})
		case app.Stage == "rejected" || app.Stage == "withdrawn":
			return nil, apperr.Validation(apperr.FieldErrors{{Field: field, Message: app.CandidateName + " is " + app.Stage + " on this job"}})
		}
		no, err := s.deps.Consent.DoNotContact(ctx, tx, app.CandidateID)
		if err != nil {
			return nil, err
		}
		if no {
			return nil, apperr.New(api.ErrorCodeSHORTLISTDONOTCONTACT,
				app.CandidateName+" withdrew consent, so they cannot be shared with a client. See Data protection on their page.").
				WithDetail("candidateId", app.CandidateID)
		}
		cv, reason, err := s.deps.CVs.ForClient(ctx, tx, p, app.CandidateID, pk.DocumentID)
		if e, ok := apperr.As(err); ok && e.Code == api.ErrorCodeNOTFOUND {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: fmt.Sprintf("candidates.%d.documentId", i), Message: "no such CV"}})
		}
		if err != nil {
			return nil, err
		}
		if reason != "" {
			return nil, apperr.New(api.ErrorCodeSHORTLISTCVNOTSHAREABLE, "The CV chosen for "+app.CandidateName+" "+reason+".").
				WithDetail("candidateId", app.CandidateID)
		}
		out[i] = Candidate{
			ApplicationID: pk.ApplicationID, CandidateID: app.CandidateID, CandidateName: app.CandidateName, DocumentID: pk.DocumentID,
			Blind: cv.Blind, Label: Label(cv, app.CandidateName), Note: pk.Note,
		}
	}
	return out, nil
}

// List returns a job's links, newest first, with the client's answers.
func (s *Service) List(ctx context.Context, p authz.Principal, jobID string) ([]Shortlist, error) {
	var out []Shortlist
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.deps.Jobs.Job(ctx, tx, jobID); err != nil {
			return err
		}
		rows, err := store.New(tx).JobShortlists(ctx, jobID)
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, rows)
		return err
	})
	return out, err
}

// Revoke stops a link working at once.
func (s *Service) Revoke(ctx context.Context, p authz.Principal, id string) (Shortlist, error) {
	var out Shortlist
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.LockShortlist(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("shortlist link")
		}
		if err != nil {
			return err
		}
		if row.RevokedAt == nil {
			if row, err = q.RevokeShortlist(ctx, id); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "client_shortlist.revoked", EntityType: "job", EntityID: row.JobID, Changes: map[string]any{"shortlistId": id},
			}); err != nil {
				return err
			}
		}
		views, err := s.views(ctx, tx, []store.ClientShortlist{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// views adds each link's candidates (those the user may see), their names
// and who made it.
func (s *Service) views(ctx context.Context, tx pgx.Tx, rows []store.ClientShortlist) ([]Shortlist, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids, users := make([]string, len(rows)), make([]string, len(rows))
	for i, r := range rows {
		ids[i], users[i] = r.ID, r.CreatedBy
	}
	items, err := store.New(tx).ShortlistCandidates(ctx, ids)
	if err != nil {
		return nil, err
	}
	candidateIDs := make([]string, len(items))
	for i, it := range items {
		candidateIDs[i] = it.CandidateID
	}
	names, err := s.deps.Candidates.Names(ctx, tx, candidateIDs)
	if err != nil {
		return nil, err
	}
	userNames, err := s.deps.Users.UserNames(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	byList := map[string][]Candidate{}
	for _, it := range items {
		byList[it.ShortlistID] = append(byList[it.ShortlistID], Candidate{
			ID: it.ID, ApplicationID: it.ApplicationID, CandidateID: it.CandidateID, CandidateName: names[it.CandidateID],
			Label: it.Label, DocumentID: it.DocumentID, Blind: it.Blind, Note: it.Note, Decision: it.Decision, Comment: it.Comment, AnsweredAt: it.AnsweredAt,
		})
	}
	now := s.now()
	out := make([]Shortlist, len(rows))
	for i, r := range rows {
		out[i] = Shortlist{
			ID: r.ID, JobID: r.JobID, Status: Status(r.ExpiresAt, r.RevokedAt, now), Message: r.Message, EmailedTo: r.EmailedTo,
			ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, RevokedAt: r.RevokedAt, OpenedAt: r.OpenedAt, LastOpenedAt: r.LastOpenedAt,
			Candidates: byList[r.ID],
		}
		if n, ok := userNames[r.CreatedBy]; ok {
			out[i].CreatedByName = &n
		}
	}
	return out, nil
}
