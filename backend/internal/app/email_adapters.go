package app

// Adapters giving the emails module what it needs from other modules, in its
// own types.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/country"
)

type emailCandidates struct{ candidates *candidates.Module }

// Detailed is a candidate with what fields show, as p may see them: salary
// only with permission (row-level security leaves it out otherwise).
func (a emailCandidates) Detailed(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (emails.Candidate, error) {
	v, err := a.candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return emails.Candidate{}, err
	}
	pr := v.Profile
	d := &emails.Details{CurrentTitle: pr.CurrentTitle, CurrentCompany: pr.CurrentCompany}
	if pr.Nationality != nil {
		d.Nationality = ptr(countryName(*pr.Nationality))
	}
	if place := joinNonEmpty(pr.Location, mapPtr(pr.ResidenceCountry, countryName)); place != "" {
		d.Residence = &place
	}
	if n := pr.NoticePeriod; n != nil {
		d.NoticePeriod = ptr(emails.FormatNotice(n.Value, n.Unit))
	}
	if wa := pr.WorkAuthorization; wa != nil {
		d.WorkAuthorization = ptr(emails.FormatWorkAuthorization(wa.Status, wa.Basis))
	}
	if c := v.Compensation; c != nil && !v.CompensationRestricted {
		if c.Current != nil {
			d.CurrentSalary = ptr(emails.FormatSalary(c.Current.Amount, c.Current.Currency, c.Current.Period))
		}
		if e := c.Expected; e != nil {
			d.ExpectedSalary = ptr(emails.FormatRange(e.Min, e.Max, e.Currency, e.Period))
		}
	}
	return emails.Candidate{
		FullName: candidates.FullName(pr), FirstName: pr.FirstName, LastName: pr.LastName, Email: pr.Email, Details: d,
	}, nil
}

func ptr(s string) *string { return &s }

func mapPtr(p *string, f func(string) string) *string {
	if p == nil {
		return nil
	}
	return ptr(f(*p))
}

func joinNonEmpty(parts ...*string) string {
	var out []string
	for _, p := range parts {
		if p != nil && strings.TrimSpace(*p) != "" {
			out = append(out, strings.TrimSpace(*p))
		}
	}
	return strings.Join(out, ", ")
}

// countryName is a country's English name from its code ("MY" -> "Malaysia").
func countryName(code string) string {
	if name := country.Name(code); name != "" {
		return name
	}
	return code
}

// emailDocuments gives the emails module a candidate's CVs to attach.
type emailDocuments struct{ documents *documents.Module }

func (a emailDocuments) CVFile(ctx context.Context, tx pgx.Tx, id string, asPDF bool) (emails.CVFile, error) {
	doc, data, err := a.documents.CVFile(ctx, tx, id, asPDF)
	return emails.CVFile{CandidateID: doc.CandidateID, FileName: doc.FileName, ContentType: doc.ContentType, Data: data}, err
}

func (a emailCandidates) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]emails.Candidate, error) {
	briefs, err := a.candidates.Briefs(ctx, tx, ids)
	out := make(map[string]emails.Candidate, len(briefs))
	for id, b := range briefs {
		out[id] = emails.Candidate{FullName: b.Name, FirstName: b.FirstName, Email: b.Email}
	}
	return out, err
}

// emailApplications combines an application (pipeline) with its job's
// hiring contact (jobs, contacts).
type emailApplications struct {
	pipeline *pipeline.Module
	jobs     *jobs.Module
	contacts *contacts.Module
}

func (a emailApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]emails.Application, error) {
	apps, err := a.pipeline.Briefs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	jobIDs := make([]string, 0, len(apps))
	for _, b := range apps {
		jobIDs = append(jobIDs, b.JobID)
	}
	jobBriefs, err := a.jobs.Briefs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	var contactIDs []string
	for _, j := range jobBriefs {
		if j.ContactID != nil {
			contactIDs = append(contactIDs, *j.ContactID)
		}
	}
	contactBriefs, err := a.contacts.Briefs(ctx, tx, contactIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]emails.Application, len(apps))
	for id, b := range apps {
		app := emails.Application{CandidateID: b.CandidateID, JobID: b.JobID, JobTitle: b.JobTitle, CompanyName: b.CompanyName}
		if j := jobBriefs[b.JobID]; j.ContactID != nil {
			if c, ok := contactBriefs[*j.ContactID]; ok {
				app.ClientEmail, app.ClientContact = c.Email, c.Name
			}
		}
		out[id] = app
	}
	return out, nil
}

// emailInterviews reads the interviews module when called: the two modules
// use each other (drafts read interviews; reminders send email), so it is
// built after this one.
type emailInterviews struct{ modules *Modules }

func (a emailInterviews) Brief(ctx context.Context, tx pgx.Tx, id string) (emails.Interview, error) {
	b, err := a.modules.Interviews.Brief(ctx, tx, id)
	d := b.Details
	return emails.Interview{
		ApplicationID: b.ApplicationID, Title: d.Title, StartsAt: d.StartsAt, Minutes: d.Duration,
		Mode: d.Mode, Location: d.Location, Instructions: d.Instructions,
	}, err
}

// interviewMailer emails interviewers (reminders): their addresses come from
// identity, the email from the emails module.
type interviewMailer struct {
	identity *identity.Module
	emails   *emails.Module
}

func (a interviewMailer) EmailUsers(ctx context.Context, tx pgx.Tx, tenantID string, userIDs []string, m interviews.Message) (int, error) {
	contacts, err := a.identity.ActiveContacts(ctx, tx, userIDs)
	if err != nil || len(contacts) == 0 {
		return 0, err
	}
	to := make([]string, 0, len(contacts))
	for _, id := range userIDs {
		if c, ok := contacts[id]; ok {
			to = append(to, c.Email)
		}
	}
	return len(to), a.emails.Notify(ctx, tx, tenantID, emails.Notice{To: to, Subject: m.Subject, Body: m.Body, ApplicationID: &m.ApplicationID})
}

// identityMailer sends identity's invitations and password resets through the
// emails module (built after identity, so read when called).
type identityMailer struct{ modules *Modules }

func (a identityMailer) Notify(ctx context.Context, tx pgx.Tx, tenantID string, to []string, subject, body string) error {
	return a.modules.Emails.Notify(ctx, tx, tenantID, emails.Notice{To: to, Subject: subject, Body: body})
}

// emailContacts gives the emails module a client's contact to write to.
type emailContacts struct {
	contacts  *contacts.Module
	companies *companies.Module
}

func (a emailContacts) Contact(ctx context.Context, tx pgx.Tx, id string) (emails.Contact, error) {
	briefs, err := a.contacts.Briefs(ctx, tx, []string{id})
	if err != nil {
		return emails.Contact{}, err
	}
	b, ok := briefs[id]
	if !ok {
		return emails.Contact{}, apperr.NotFound("contact")
	}
	names, err := a.companies.Names(ctx, tx, []string{b.CompanyID})
	if err != nil {
		return emails.Contact{}, err
	}
	first, last, _ := strings.Cut(strings.TrimSpace(b.Name), " ")
	return emails.Contact{FirstName: first, LastName: strings.TrimSpace(last), FullName: b.Name, CompanyName: names[b.CompanyID], Email: b.Email}, nil
}
