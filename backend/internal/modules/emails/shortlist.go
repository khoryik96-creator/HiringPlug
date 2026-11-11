package emails

// An email to the client about several candidates (a shortlist): the
// composer puts each one's details in the message (a block or a table row
// each, from their own values); fields left in it are the job's, its
// client's and the sender's. The email is kept on each candidate's emails
// and noted on the job.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Shortlisted is one of the candidates an email to the client is about.
type Shortlisted struct {
	CandidateVersion, CompensationVersion *int
	CandidateID                           string
	ApplicationID                         *string // their application to the job
}

func notBoth() error {
	var errs apperr.FieldErrors
	errs.Add("candidates", "an email is about several candidates or about one (candidateId, applicationId, contactId), not both")
	return errs.Err()
}

// checkShortlist checks each candidate (once each, their own application,
// and willing to be put forward) and returns the first.
func (s *Service) checkShortlist(ctx context.Context, tx pgx.Tx, p authz.Principal, list []Shortlisted) (Shortlisted, error) {
	if !p.Can(api.PermissionCandidatesRead) {
		return Shortlisted{}, apperr.New(api.ErrorCodeFORBIDDEN, "Selecting candidate details needs permission to read candidates.")
	}
	seen := map[string]bool{}
	for _, c := range list {
		if seen[c.CandidateID] {
			var errs apperr.FieldErrors
			errs.Add("candidates", "lists a candidate twice")
			return Shortlisted{}, errs.Err()
		}
		seen[c.CandidateID] = true
		candidate, err := s.deps.Candidates.Detailed(ctx, tx, p, c.CandidateID)
		if err != nil {
			return Shortlisted{}, err
		}
		if c.CandidateVersion != nil && candidate.Version != *c.CandidateVersion {
			return Shortlisted{}, apperr.StaleVersion("selected candidate details").WithDetail("candidateId", c.CandidateID)
		}
		if c.CompensationVersion != nil && candidate.CompensationVersion != *c.CompensationVersion {
			return Shortlisted{}, apperr.StaleVersion("selected candidate salary").WithDetail("candidateId", c.CandidateID)
		}
		if c.ApplicationID != nil {
			if _, err := s.application(ctx, tx, *c.ApplicationID, &c.CandidateID); err != nil {
				return Shortlisted{}, err
			}
		}
		if err := s.contactable(ctx, tx, c.CandidateID); err != nil {
			return Shortlisted{}, err
		}
	}
	return list[0], nil
}

// shortlistValues are the values for the fields left in a shortlist's
// message: the job's, its client's and the sender's (from the first
// candidate's application), never one candidate's.
func (s *Service) shortlistValues(ctx context.Context, tx pgx.Tx, p authz.Principal, first Shortlisted, loc *time.Location) (map[string]string, error) {
	values, err := s.values(ctx, tx, p, &first.CandidateID, first.ApplicationID, nil, nil, loc)
	if err != nil {
		return nil, err
	}
	for key := range values {
		if strings.HasPrefix(key, "candidate.") {
			delete(values, key)
		}
	}
	return values, nil
}

// cvAllowed: a CV may go with an email about its candidate (or one of them).
func cvAllowed(owner string, candidateID *string, list []Shortlisted) bool {
	if len(list) == 0 {
		return candidateID != nil && owner == *candidateID
	}
	for _, c := range list {
		if c.CandidateID == owner {
			return true
		}
	}
	return false
}

// keepShortlist keeps the email on each candidate's emails, in order, and
// notes it on each job it is about.
func (s *Service) keepShortlist(ctx context.Context, tx pgx.Tx, emailID string, list []Shortlisted, to []string) error {
	if len(list) == 0 {
		return nil
	}
	q := store.New(tx)
	var appIDs []string
	for i, c := range list {
		if err := q.InsertEmailCandidate(ctx, store.InsertEmailCandidateParams{
			EmailID: emailID, CandidateID: c.CandidateID, ApplicationID: c.ApplicationID, Position: int16(i + 1), //nolint:gosec // at most 10
		}); err != nil {
			return err
		}
		if c.ApplicationID != nil {
			appIDs = append(appIDs, *c.ApplicationID)
		}
	}
	apps, err := s.deps.Applications.Briefs(ctx, tx, appIDs)
	if err != nil {
		return err
	}
	perJob := map[string]int{}
	var jobs []string
	for _, id := range appIDs {
		job := apps[id].JobID
		if job == "" {
			continue
		}
		if perJob[job] == 0 {
			jobs = append(jobs, job)
		}
		perJob[job]++
	}
	for _, job := range jobs {
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "email.shortlist_sent", EntityType: "job", EntityID: job,
			Changes: map[string]any{"emailId": emailID, "candidates": perJob[job], "to": to},
		}); err != nil {
			return err
		}
	}
	return nil
}
