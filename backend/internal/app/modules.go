package app

// This file is the one place modules are wired together. To add a module
// (`task new:module -- name` does all of this for you):
//  1. add a field to Modules and construct it in NewModules;
//  2. embed its HTTP handlers in Server (if it has endpoints);
//  3. add its JobHandlers in JobHandlers (if it handles jobs).
// The compiler then checks that every endpoint in the contract has a handler,
// and the worker refuses to start if a job kind has none.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/careers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	// scaffold:imports
)

// Modules holds every module, wired together.
type Modules struct {
	Identity   *identity.Module
	Candidates *candidates.Module
	Documents  *documents.Module
	Companies  *companies.Module
	Contacts   *contacts.Module
	Jobs       *jobs.Module
	Pipeline   *pipeline.Module
	Notes      *notes.Module
	Careers    *careers.Module
	Interviews *interviews.Module
	Emails     *emails.Module
	Placements *placements.Module
	// scaffold:fields
}

// NewModules builds every module. A module may receive another module (or a
// small interface it satisfies) to call its public API, as Documents does
// with Candidates.
func NewModules(d Deps) *Modules {
	m := &Modules{}
	m.Identity = identity.New(d.Pool, d.Verifier)
	m.Candidates = candidates.New(d.Pool, d.Config.DefaultPhoneArea)
	m.Documents = documents.New(d.Pool, d.Files, m.Candidates, d.Config.MaxUploadBytes)
	m.Companies = companies.New(d.Pool)
	m.Contacts = contacts.New(d.Pool, m.Companies, d.MailServers)
	m.Jobs = jobs.New(d.Pool, m.Companies, jobContacts{m.Contacts})
	m.Pipeline = pipeline.New(d.Pool, pipelineCandidates{m.Candidates}, pipelineJobs{m.Jobs}, m.Companies)
	m.Notes = notes.New(d.Pool, notes.Subjects{
		"candidate": m.Candidates.Exists, "job": m.Jobs.Exists, "company": m.Companies.Exists, "contact": m.Contacts.Exists,
	}, m.Identity)
	m.Careers = careers.New(d.Pool, careers.Deps{
		Tenants: m.Identity, Ads: careersAds{m.Jobs}, Candidates: careersCandidates{m.Candidates},
		CVs: m.Documents, Pipeline: m.Pipeline,
	}, d.Config.MaxUploadBytes, d.Config.PublicURL)
	m.Emails = emails.New(d.Pool, emails.Deps{
		Candidates: emailCandidates{m.Candidates}, Applications: emailApplications{m.Pipeline, m.Jobs, m.Contacts},
		Interviews: emailInterviews{m}, Tenants: m.Identity,
	}, d.Mail)
	m.Interviews = interviews.New(d.Pool, interviewApplications{m.Pipeline}, m.Identity, interviewMailer{m.Identity, m.Emails},
		interviews.Settings{TimeZone: d.Config.TimeZone, PublicURL: d.Config.PublicURL})
	m.Placements = placements.New(d.Pool, placementApplications{m.Pipeline}, m.Identity)
	// scaffold:constructors
	// A merged duplicate's CVs, jobs, notes and emails follow it.
	m.Candidates.MoveRecordsWith(m.Documents, m.Pipeline, m.Notes, m.Emails)
	return m
}

// Each module's handler type is called HTTP; these aliases give the embedded
// fields distinct names.
type (
	identityHTTP   = identity.HTTP
	candidatesHTTP = candidates.HTTP
	documentsHTTP  = documents.HTTP
	companiesHTTP  = companies.HTTP
	contactsHTTP   = contacts.HTTP
	jobsHTTP       = jobs.HTTP
	pipelineHTTP   = pipeline.HTTP
	notesHTTP      = notes.HTTP
	careersHTTP    = careers.HTTP
	interviewsHTTP = interviews.HTTP
	emailsHTTP     = emails.HTTP
	placementsHTTP = placements.HTTP
	// scaffold:aliases
)

// Server combines every module's handlers into the single interface the
// contract defines. If an endpoint in contracts/ has no handler, or two
// modules implement the same one, this file does not compile.
type Server struct {
	*identityHTTP
	*candidatesHTTP
	*documentsHTTP
	*companiesHTTP
	*contactsHTTP
	*jobsHTTP
	*pipelineHTTP
	*notesHTTP
	*careersHTTP
	*interviewsHTTP
	*emailsHTTP
	*placementsHTTP
	// scaffold:embeds
}

var _ api.StrictServerInterface = (*Server)(nil)

// Server returns the composed HTTP handlers.
func (m *Modules) Server() *Server {
	return &Server{
		identityHTTP:   m.Identity.HTTP,
		candidatesHTTP: m.Candidates.HTTP,
		documentsHTTP:  m.Documents.HTTP,
		companiesHTTP:  m.Companies.HTTP,
		contactsHTTP:   m.Contacts.HTTP,
		jobsHTTP:       m.Jobs.HTTP,
		pipelineHTTP:   m.Pipeline.HTTP,
		notesHTTP:      m.Notes.HTTP,
		careersHTTP:    m.Careers.HTTP,
		interviewsHTTP: m.Interviews.HTTP,
		emailsHTTP:     m.Emails.HTTP,
		placementsHTTP: m.Placements.HTTP,
		// scaffold:server-fields
	}
}

// JobHandlers merges every module's handlers for the core queue.
func (m *Modules) JobHandlers() (jobqueue.Handlers, error) {
	all := jobqueue.Handlers{}
	for _, handlers := range []jobqueue.Handlers{
		m.Documents.JobHandlers(),
		m.Emails.JobHandlers(),
		m.Interviews.JobHandlers(),
		// scaffold:job-handlers
	} {
		for kind, h := range handlers {
			if _, dup := all[kind]; dup {
				return nil, fmt.Errorf("two modules handle job kind %q", kind)
			}
			all[kind] = h
		}
	}
	return all, nil
}

// jobContacts gives the jobs module what it needs from contacts, in its own
// types, so neither module imports the other.
type jobContacts struct{ contacts *contacts.Module }

func (a jobContacts) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]jobs.ContactInfo, error) {
	briefs, err := a.contacts.Briefs(ctx, tx, ids)
	out := make(map[string]jobs.ContactInfo, len(briefs))
	for id, b := range briefs {
		out[id] = jobs.ContactInfo{Name: b.Name, CompanyID: b.CompanyID}
	}
	return out, err
}

// pipelineCandidates and pipelineJobs give the pipeline module what it needs,
// in its own types.
type pipelineCandidates struct{ *candidates.Module }

func (a pipelineCandidates) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]pipeline.CandidateInfo, error) {
	briefs, err := a.Briefs(ctx, tx, ids)
	out := make(map[string]pipeline.CandidateInfo, len(briefs))
	for id, b := range briefs {
		out[id] = pipeline.CandidateInfo{Name: b.Name, Title: b.CurrentTitle}
	}
	return out, err
}

type pipelineJobs struct{ *jobs.Module }

func (a pipelineJobs) OpenJobs(ctx context.Context, tx pgx.Tx, limit int) ([]pipeline.OpenJob, error) {
	open, err := a.Open(ctx, tx, limit)
	out := make([]pipeline.OpenJob, len(open))
	for i, j := range open {
		out[i] = pipeline.OpenJob{ID: j.ID, Title: j.Title, CompanyID: j.CompanyID}
	}
	return out, err
}

func (a pipelineJobs) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]pipeline.JobInfo, error) {
	briefs, err := a.Briefs(ctx, tx, ids)
	out := make(map[string]pipeline.JobInfo, len(briefs))
	for id, b := range briefs {
		out[id] = pipeline.JobInfo{Title: b.Title, CompanyID: b.CompanyID}
	}
	return out, err
}
