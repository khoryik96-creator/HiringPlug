package contacts

// LinkedIn checks: where a company's contacts work, read from their LinkedIn
// profiles by the HiringPlug LinkedIn companion, a browser extension in the
// recruiter's own browser (the server never reads LinkedIn). The server makes
// one check per contact, bound to the contact, their profile and the
// company's LinkedIn page; only whoever asked may answer it, within 15
// minutes, with the dates of the contact's time at that exact company.
// "Their time there ended, with no current role there" makes a contact who
// works there one who has left; anything unclear changes nothing.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// LinkedinCheckLifetime is how long a LinkedIn check waits for its answer.
const LinkedinCheckLifetime = 15 * time.Minute

const maxLinkedinChecks = 200

// LinkedinCheckRequest is a check for the companion to answer.
type LinkedinCheckRequest struct{ ID, ContactID, ContactName, ProfileURL string }

// LinkedinChecks are the checks started for a company (or one contact).
type LinkedinChecks struct {
	CompanyURL string
	Checks     []LinkedinCheckRequest
	Skipped    []LinkedinCheckRequest // no usable profile address
}

// LinkedinCheck is a check and its answer.
type LinkedinCheck struct {
	ID, ContactID, Status string // pending, observed, failed, expired
	Joined, Left, Error   *string
	Current               *bool
	MarkedLeft            bool // it set the contact to "has left"
	CreatedAt             time.Time
	CheckedAt             *time.Time
}

func linkedinCheckFrom(r store.ContactLinkedinCheck) LinkedinCheck {
	return LinkedinCheck{ID: r.ID, ContactID: r.ContactID, Status: r.Status, Joined: r.Joined, Left: r.LeftOn, Error: r.Error,
		Current: r.CurrentJob, MarkedLeft: r.MarkedLeft, CreatedAt: r.CreatedAt, CheckedAt: r.CheckedAt}
}

// LinkedinAnswer is what the companion read (or why it could not).
type LinkedinAnswer struct {
	ProfileURL, CompanyURL, LinkedinCompanyID string
	Joined, Left                              string
	Current                                   *bool
	Error                                     string
}

// StartLinkedinChecks starts a check for each of a company's contacts with a
// LinkedIn profile, or for one contact.
func (s *Service) StartLinkedinChecks(ctx context.Context, p authz.Principal, companyID, contactID *string) (LinkedinChecks, error) {
	var out LinkedinChecks
	if (companyID == nil) == (contactID == nil) {
		var errs apperr.FieldErrors
		errs.Add("companyId", "give a company or a contact")
		return out, errs.Err()
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var people []store.CompanyContactsForLinkedinRow
		if contactID != nil {
			row, err := getRow(ctx, q, *contactID)
			if err != nil {
				return err
			}
			companyID = &row.CompanyID
			people = append(people, store.CompanyContactsForLinkedinRow{ID: row.ID, FirstName: row.FirstName, LastName: row.LastName, LinkedinUrl: row.LinkedinUrl})
		} else {
			if err := s.checkCompany(ctx, tx, *companyID); err != nil {
				return err
			}
			var err error
			if people, err = q.CompanyContactsForLinkedin(ctx, *companyID); err != nil {
				return err
			}
		}
		page, err := s.companies.LinkedinURL(ctx, tx, *companyID)
		if err != nil {
			return err
		}
		if out.CompanyURL = CompanyPageURL(page); out.CompanyURL == "" {
			var errs apperr.FieldErrors
			errs.Add("companyId", "add the company's LinkedIn page first (linkedin.com/company/...)")
			return errs.Err()
		}
		for _, person := range people {
			r := LinkedinCheckRequest{ContactID: person.ID, ContactName: FullName(person.FirstName, person.LastName)}
			if person.LinkedinUrl != nil {
				r.ProfileURL = ProfileURL(*person.LinkedinUrl)
			}
			if r.ProfileURL == "" || len(out.Checks) == maxLinkedinChecks {
				out.Skipped = append(out.Skipped, r)
				continue
			}
			row, err := q.InsertLinkedinCheck(ctx, store.InsertLinkedinCheckParams{
				ContactID: person.ID, CompanyID: *companyID, ProfileUrl: r.ProfileURL, CompanyUrl: out.CompanyURL,
			})
			if err != nil {
				return err
			}
			r.ID = row.ID
			out.Checks = append(out.Checks, r)
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contact.linkedin_checks_started", EntityType: "company", EntityID: *companyID,
			Changes: map[string]any{"checks": len(out.Checks)}})
	})
	return out, err
}

// AnswerLinkedinCheck saves what the companion read for a check.
func (s *Service) AnswerLinkedinCheck(ctx context.Context, p authz.Principal, checkID string, a LinkedinAnswer) (LinkedinCheck, error) {
	var out LinkedinCheck
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		check, err := q.GetLinkedinCheck(ctx, checkID)
		if database.IsNoRows(err) || (err == nil && check.RequestedBy != p.UserID) {
			return apperr.NotFound("LinkedIn check") // someone else's check is not theirs to answer
		}
		if err != nil {
			return err
		}
		if check.Status == "pending" && time.Since(check.CreatedAt) > LinkedinCheckLifetime {
			if err := q.ExpireLinkedinCheck(ctx, checkID); err != nil {
				return err
			}
			check.Status = "expired"
		}
		if check.Status != "pending" {
			return apperr.New(api.ErrorCodeCONTACTLINKEDINCHECKCLOSED, "This LinkedIn check was already answered, or expired. Check again.")
		}
		params := store.SaveLinkedinCheckParams{ID: checkID, Status: "failed"}
		if a.Error != "" {
			params.Error = ptr(clip(a.Error, 300))
		} else {
			var errs apperr.FieldErrors
			if a.ProfileURL != check.ProfileUrl {
				errs.Add("profileUrl", "is not the profile of this check")
			}
			if a.CompanyURL != check.CompanyUrl {
				errs.Add("companyUrl", "is not the company of this check")
			}
			if !linkedinCompanyIDRe.MatchString(a.LinkedinCompanyID) {
				errs.Add("linkedinCompanyId", "LinkedIn's number for the company is needed")
			}
			if !ValidEmployment(a.Joined, a.Left, a.Current) {
				errs.Add("joined", "the dates do not make sense: current (joined), former (joined and left) or neither")
			}
			if err := errs.Err(); err != nil {
				return err
			}
			params.Status, params.CurrentJob, params.LinkedinCompanyID = "observed", a.Current, &a.LinkedinCompanyID
			params.Joined, params.LeftOn = optional(a.Joined), optional(a.Left)
			if a.Current != nil && !*a.Current {
				if params.MarkedLeft, err = s.markLeft(ctx, q, check); err != nil {
					return err
				}
			}
		}
		saved, err := q.SaveLinkedinCheck(ctx, params)
		if err != nil {
			return err
		}
		out = linkedinCheckFrom(saved)
		changes := map[string]any{"status": out.Status, "current": out.Current, "joined": out.Joined, "left": out.Left}
		if out.MarkedLeft {
			changes["contactStatus"] = map[string]any{"from": "active", "to": "left"}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contact.linkedin_checked", EntityType: "contact", EntityID: check.ContactID, Changes: changes})
	})
	return out, err
}

// markLeft sets a contact to "has left" if they still work there per HiringPlug,
// at the same company, with the profile the check read.
func (s *Service) markLeft(ctx context.Context, q *store.Queries, check store.ContactLinkedinCheck) (bool, error) {
	row, err := q.GetContact(ctx, check.ContactID)
	if err != nil {
		return false, err
	}
	if row.LinkedinUrl == nil || ProfileURL(*row.LinkedinUrl) != check.ProfileUrl {
		return false, nil // the contact's profile changed since: not this person's finding
	}
	_, err = q.MarkContactLeft(ctx, store.MarkContactLeftParams{ID: check.ContactID, CompanyID: check.CompanyID})
	if database.IsNoRows(err) {
		return false, nil // left already, or moved to another company
	}
	return err == nil, err
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StartContactLinkedinChecks handles POST /v1/contacts/linkedin-checks.
func (h *HTTP) StartContactLinkedinChecks(ctx context.Context, req api.StartContactLinkedinChecksRequestObject) (api.StartContactLinkedinChecksResponseObject, error) {
	started, err := h.svc.StartLinkedinChecks(ctx, authz.MustFromContext(ctx), req.Body.CompanyId, req.Body.ContactId)
	if err != nil {
		return nil, err
	}
	out := api.StartContactLinkedinChecks201JSONResponse{CompanyUrl: started.CompanyURL,
		Checks: make([]api.ContactLinkedinCheckRequest, len(started.Checks)), Skipped: make([]api.ContactLinkedinCheckSkipped, len(started.Skipped))}
	for i, r := range started.Checks {
		out.Checks[i] = api.ContactLinkedinCheckRequest{Id: r.ID, ContactId: r.ContactID, ContactName: r.ContactName, ProfileUrl: r.ProfileURL}
	}
	for i, r := range started.Skipped {
		out.Skipped[i] = api.ContactLinkedinCheckSkipped{ContactId: r.ContactID, ContactName: r.ContactName}
	}
	return out, nil
}

// AnswerContactLinkedinCheck handles POST /v1/contacts/linkedin-checks/{checkId}/answer.
func (h *HTTP) AnswerContactLinkedinCheck(ctx context.Context, req api.AnswerContactLinkedinCheckRequestObject) (api.AnswerContactLinkedinCheckResponseObject, error) {
	b := req.Body
	a := LinkedinAnswer{ProfileURL: deref(b.ProfileUrl), CompanyURL: deref(b.CompanyUrl), LinkedinCompanyID: deref(b.LinkedinCompanyId),
		Joined: deref(b.Joined), Left: deref(b.Left), Error: deref(b.Error)}
	if v, err := b.Current.Get(); err == nil {
		a.Current = &v
	}
	row, err := h.svc.AnswerLinkedinCheck(ctx, authz.MustFromContext(ctx), req.CheckId, a)
	if err != nil {
		return nil, err
	}
	return api.AnswerContactLinkedinCheck200JSONResponse(linkedinCheckToAPI(row)), nil
}

func linkedinCheckToAPI(r LinkedinCheck) api.ContactLinkedinCheck {
	return api.ContactLinkedinCheck{
		Id: r.ID, ContactId: r.ContactID, Status: api.ContactLinkedinCheckStatus(r.Status), Joined: c.OrNull(r.Joined), Left: c.OrNull(r.Left),
		Current: c.OrNull(r.Current), MarkedLeft: r.MarkedLeft, Error: c.OrNull(r.Error), CreatedAt: r.CreatedAt, CheckedAt: c.OrNull(r.CheckedAt),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func linkedinEmploymentToAPI(l *LinkedinEmployment) *api.LinkedinEmployment {
	if l == nil {
		return nil
	}
	return &api.LinkedinEmployment{Current: c.OrNull(l.Current), Joined: c.OrNull(l.Joined), Left: c.OrNull(l.Left), CheckedAt: l.CheckedAt}
}
