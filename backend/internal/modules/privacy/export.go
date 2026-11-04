package privacy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// DataSource gathers what the other modules hold about a candidate, as the
// given user may see it (NotFound for a candidate that does not exist).
type DataSource interface {
	About(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID string) (Held, error)
}

// Export is a copy of everything held about a candidate, for an access
// request.
type Export struct {
	CandidateID, CandidateName, Company string
	ExportedAt                          time.Time
	Sections                            []Section
	NotIncluded                         []string
}

// Words for the copy a candidate receives.
var (
	kindWords = map[string]string{
		"access": "A copy of their data", "correction": "Correct their data", "withdraw_consent": "Stop using their data and contacting them",
		"removal": "Remove their data", "other": "Other",
	}
	statusWords = map[string]string{"open": "Open", "done": "Done", "declined": "Declined", Given: "Given", Withdrawn: "Withdrawn"}
	methodWords = map[string]string{
		MethodCareersForm: "Careers page application form", "email": "Email", "phone": "Phone", "in_person": "In person",
		"document": "Signed document", MethodRequest: "On their request", "other": "Other",
	}
)

// Export returns a copy of everything held about a candidate, as the user
// may see it, with what was left out. The export is recorded in the audit
// log (who, when; not what).
func (s *Service) Export(ctx context.Context, p authz.Principal, candidateID string) (Export, error) {
	out := Export{CandidateID: candidateID, ExportedAt: s.now().UTC()}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		held, err := s.deps.Data.About(ctx, tx, p, candidateID)
		if err != nil {
			return err
		}
		if out.Company, err = s.deps.Tenants.TenantName(ctx, tx, p.TenantID); err != nil {
			return err
		}
		q := store.New(tx)
		var consent *Consent
		if row, err := q.GetConsent(ctx, candidateID); err == nil {
			c := consentFromRow(row)
			consent = &c
		} else if !database.IsNoRows(err) {
			return err
		}
		rows, err := q.ListCandidateRequests(ctx, candidateID)
		if err != nil {
			return err
		}
		requests := make([]Request, len(rows))
		for i, r := range rows {
			requests[i] = requestFromRow(r)
		}
		out.CandidateName, out.NotIncluded = held.Name, held.NotIncluded
		if out.NotIncluded == nil {
			out.NotIncluded = []string{}
		}
		out.Sections = append(held.Sections, PrivacySections(consent, requests)...)
		return audit.Record(ctx, tx, audit.Entry{
			Action: "privacy.exported", EntityType: "candidate", EntityID: candidateID,
			Changes: map[string]any{"sections": len(out.Sections)},
		})
	})
	return out, err
}

// PrivacySections are the candidate's consent and requests, in plain words.
func PrivacySections(consent *Consent, requests []Request) []Section {
	c := Section{Title: "Consent", Items: []Item{}}
	if consent == nil {
		c.Items = append(c.Items, Item{{"Consent", "Not recorded"}})
	} else {
		item := Item{{"Consent", statusWords[consent.Status]}, {"How", methodWords[consent.Method]}, {"Date", consent.Day.Format(time.DateOnly)}}
		if consent.Note != nil {
			item = append(item, Field{"Note", *consent.Note})
		}
		c.Items = append(c.Items, item)
	}
	r := Section{Title: "Requests about their data", Items: []Item{}}
	for _, req := range requests {
		item := Item{{"Request", kindWords[req.Kind]}, {"Received", req.ReceivedOn.Format(time.DateOnly)}, {"Status", statusWords[req.Status]}}
		if req.Details != nil {
			item = append(item, Field{"Details", *req.Details})
		}
		if req.Outcome != nil {
			item = append(item, Field{"Outcome", *req.Outcome})
		}
		r.Items = append(r.Items, item)
	}
	return []Section{c, r}
}
