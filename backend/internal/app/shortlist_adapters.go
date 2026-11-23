package app

// Adapters giving the shortlists module (client shortlist links) jobs,
// applications, CVs and email, in its own types.

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/shortlists"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// shortlistJobs gives a job's title and hiring contact (jobs, contacts).
type shortlistJobs struct {
	jobs     *jobs.Module
	contacts *contacts.Module
}

func (a shortlistJobs) Job(ctx context.Context, tx pgx.Tx, id string) (shortlists.Job, error) {
	briefs, err := a.jobs.Briefs(ctx, tx, []string{id})
	if err != nil {
		return shortlists.Job{}, err
	}
	b, ok := briefs[id]
	if !ok {
		return shortlists.Job{}, apperr.NotFound("job")
	}
	out := shortlists.Job{Title: b.Title}
	if b.ContactID != nil {
		cs, err := a.contacts.Briefs(ctx, tx, []string{*b.ContactID})
		if err != nil {
			return out, err
		}
		if c, ok := cs[*b.ContactID]; ok {
			out.ContactName, out.ContactEmail = c.Name, c.Email
		}
	}
	return out, nil
}

// shortlistApplications reads and moves applications (pipeline).
type shortlistApplications struct{ pipeline *pipeline.Module }

func (a shortlistApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]shortlists.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	out := make(map[string]shortlists.Application, len(briefs))
	for id, b := range briefs {
		out[id] = shortlists.Application{JobID: b.JobID, CandidateID: b.CandidateID, CandidateName: b.CandidateName, Stage: b.Stage}
	}
	return out, err
}

func (a shortlistApplications) Approved(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	return a.pipeline.ClientApproved(ctx, tx, id)
}

func (a shortlistApplications) Rejected(ctx context.Context, tx pgx.Tx, id, comment string) (string, error) {
	return a.pipeline.ClientRejectedWith(ctx, tx, id, comment)
}

// shortlistCVs checks CVs for clients (cvformats) and reads them (documents).
type shortlistCVs struct {
	cvformats *cvformats.Module
	documents *documents.Module
}

func (a shortlistCVs) ForClient(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID, documentID string) (shortlists.ClientCV, string, error) {
	cv, reason, err := a.cvformats.ForClient(ctx, tx, p, candidateID, documentID)
	return shortlists.ClientCV{Blind: cv.Blind, Reference: cv.Reference}, reason, err
}

// File is the CV's PDF when it has one, else the Word file.
func (a shortlistCVs) File(ctx context.Context, tx pgx.Tx, documentID string) (shortlists.CVFile, error) {
	d, data, err := a.documents.CVFile(ctx, tx, documentID, true)
	if e, ok := apperr.As(err); ok && e.Code == api.ErrorCodeNOTFOUND {
		d, data, err = a.documents.CVFile(ctx, tx, documentID, false)
	}
	return shortlists.CVFile{FileName: d.FileName, Data: data}, err
}

// FileName is the name File would give: the PDF's when it is made.
func (a shortlistCVs) FileName(ctx context.Context, tx pgx.Tx, documentID string) (string, error) {
	d, _, err := a.documents.SourceCV(ctx, tx, documentID)
	if err != nil {
		return "", err
	}
	if d.PdfStatus != nil && *d.PdfStatus == "ready" {
		return strings.TrimSuffix(d.FileName, filepath.Ext(d.FileName)) + ".pdf", nil
	}
	return d.FileName, nil
}

// shortlistMailer queues the email with the link to the hiring contact.
type shortlistMailer struct{ emails *emails.Module }

func (a shortlistMailer) Send(ctx context.Context, tx pgx.Tx, tenantID string, m shortlists.Mail) error {
	from := m.From
	return a.emails.Notify(ctx, tx, tenantID, emails.Notice{To: []string{m.To}, Subject: m.Subject, Body: m.Body, SentBy: &from, SenderName: m.FromName})
}

// newShortlists wires the shortlists module to the modules it uses.
func newShortlists(d Deps, m *Modules) *shortlists.Module {
	return shortlists.New(d.Pool, shortlists.Deps{
		Jobs: shortlistJobs{m.Jobs, m.Contacts}, Applications: shortlistApplications{m.Pipeline}, Candidates: privacyCandidates{m.Candidates},
		CVs: shortlistCVs{m.Cvformats, m.Documents}, Consent: m.Privacy, Users: m.Identity, Mailer: shortlistMailer{m.Emails},
	}, d.Config.PublicURL, d.Config.TimeZone)
}
