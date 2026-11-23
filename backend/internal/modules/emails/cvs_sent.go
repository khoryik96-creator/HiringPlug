package emails

// CVs sent: whose CVs an email carries to someone other than that candidate
// (a client, a hiring manager), kept on the email for each user's activity
// in reports.

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
)

// CVsSentTo returns the candidates whose CVs are among cvs, each once, less
// those the email goes to (recipients: candidates with an address it is sent
// to). Pure: unit tested.
func CVsSentTo(cvs []CVFile, recipients []string) []string {
	var out []string
	for _, cv := range cvs {
		if cv.CandidateID == "" || slices.Contains(recipients, cv.CandidateID) || slices.Contains(out, cv.CandidateID) {
			continue
		}
		out = append(out, cv.CandidateID)
	}
	return out
}

func (s *Service) recordCVsSent(ctx context.Context, tx pgx.Tx, emailID string, cvs []CVFile, to, cc, bcc []string) error {
	if len(cvs) == 0 {
		return nil
	}
	recipients, err := s.deps.Candidates.IDsByEmails(ctx, tx, slices.Concat(to, cc, bcc))
	if err != nil {
		return err
	}
	ids := CVsSentTo(cvs, recipients)
	if len(ids) == 0 {
		return nil
	}
	return store.New(tx).SetEmailCVCandidates(ctx, store.SetEmailCVCandidatesParams{ID: emailID, CandidateIds: ids})
}

// CVEmailed is a candidate's CV emailed by a user to someone else.
type CVEmailed struct {
	SenderID, CandidateID string
	At                    time.Time
}

// CVsEmailed returns the CVs emailed in [from, to) (not failed), one per
// candidate per email, in the caller's transaction.
func (m *Module) CVsEmailed(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CVEmailed, error) {
	rows, err := store.New(tx).CVsEmailedBetween(ctx, store.CVsEmailedBetweenParams{FromTime: from, ToTime: to})
	var out []CVEmailed
	for _, r := range rows {
		for _, id := range r.CvCandidateIds {
			out = append(out, CVEmailed{SenderID: r.SentBy, CandidateID: id, At: r.CreatedAt})
		}
	}
	return out, err
}
