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
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/careers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/intake"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/market"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/reports"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	// scaffold:imports
)

// Modules holds every module, wired together.
type Modules struct {
	Identity     *identity.Module
	Candidates   *candidates.Module
	Documents    *documents.Module
	Companies    *companies.Module
	Contacts     *contacts.Module
	Jobs         *jobs.Module
	Pipeline     *pipeline.Module
	Notes        *notes.Module
	Careers      *careers.Module
	Interviews   *interviews.Module
	Emails       *emails.Module
	Placements   *placements.Module
	Reports      *reports.Module
	Offers       *offers.Module
	Integrations *integrations.Module
	Leads        *leads.Module
	Cvformats    *cvformats.Module
	Assistant    *assistant.Module
	Matching     *matching.Module
	Privacy      *privacy.Module
	Settings     *settings.Module
	Intake       *intake.Module
	Market       *market.Module
	AIProviders  *aiproviders.Module
	Scheduling   *scheduling.Module
	Industries   *industries.Module
	Automations  *automations.Module
	// scaffold:fields
}

// NewModules builds every module. A module may receive another module (or a
// small interface it satisfies) to call its public API, as Documents does
// with Candidates.
func NewModules(d Deps) *Modules {
	m := &Modules{}
	m.Identity = identity.New(d.Pool, d.Verifier, identityMailer{m}, d.Config.PublicURL)
	m.Settings = settings.New(d.Pool)
	m.Settings.UseBackups(settings.BackupSetup{Dir: d.Config.BackupDir, OperatorTenant: d.Config.BackupTenant})
	m.Industries = industries.New(d.Pool)
	m.AIProviders = aiproviders.New(d.Pool, aiproviders.Settings{ServerOn: d.Config.AI.Enabled, KeysSecret: d.Config.AI.KeysSecret})
	m.Candidates = candidates.New(d.Pool, d.Config.DefaultPhoneArea, m.Identity, m.Settings)
	m.Documents = documents.New(d.Pool, d.Files, documentCandidates{m.Candidates}, d.Config.MaxUploadBytes)
	m.Candidates.SearchCVsWith(m.Documents) // advanced search looks in their CVs
	m.Companies = companies.New(d.Pool, d.Config.AllowPrivateNetworks, d.Files, d.Config.MaxUploadBytes, m.Identity)
	m.Contacts = contacts.New(d.Pool, m.Companies, m.Settings, d.MailServers, d.Mailboxes, d.Files, d.Config.MaxUploadBytes)
	m.Jobs = jobs.New(d.Pool, m.Companies, jobContacts{m.Contacts}, m.Identity, m.Settings, d.Files, d.Config.MaxUploadBytes)
	m.Pipeline = pipeline.New(d.Pool, pipelineCandidates{m.Candidates}, pipelineJobs{m.Jobs}, m.Companies)
	m.Pipeline.NameUsersWith(m.Identity)
	m.Notes = notes.New(d.Pool, notes.Subjects{
		"candidate": m.Candidates.Exists, "job": m.Jobs.Exists, "company": m.Companies.Exists, "contact": m.Contacts.Exists,
	}, m.Identity)
	// Before careers and emails, which record and check consent with it.
	m.Privacy = privacy.New(d.Pool, privacy.Deps{
		Candidates: privacyCandidates{m.Candidates}, Users: m.Identity, Tenants: m.Identity, Data: privacyData{m},
	}, d.Config.TimeZone)
	m.Careers = careers.New(d.Pool, careers.Deps{
		Tenants: m.Identity, Ads: careersAds{m.Jobs}, Candidates: careersCandidates{m.Candidates},
		CVs: m.Documents, Pipeline: m.Pipeline, Consent: m.Privacy,
	}, d.Config.MaxUploadBytes, d.Config.PublicURL)
	m.Emails = emails.New(d.Pool, emails.Deps{
		Candidates: emailCandidates{m.Candidates, m.Industries, m.Settings}, Applications: emailApplications{m.Pipeline, m.Jobs, m.Contacts},
		Interviews: emailInterviews{m}, Tenants: m.Identity, Consent: m.Privacy,
		Documents: emailDocuments{m.Documents}, Policy: m.Settings, Contacts: emailContacts{m.Contacts, m.Companies},
		Undeliverable: emailUndeliverable{m.Contacts}, Bounces: m.Contacts,
	}, d.Mail, d.Files)
	mode := mail.DeliveryMode(d.Config.SMTP.Addr)
	m.Emails.ConnectMailboxesWith(emails.ConnectionSettings{
		KeysSecret: d.Config.MailConnections.KeysSecret, PublicURL: d.Config.PublicURL, ServerMode: mode, AllowPrivate: d.Config.AllowPrivateNetworks,
		Microsoft: emails.OAuthApplication{ClientID: d.Config.MailConnections.MicrosoftClientID, ClientSecret: d.Config.MailConnections.MicrosoftClientSecret},
		Google:    emails.OAuthApplication{ClientID: d.Config.MailConnections.GoogleClientID, ClientSecret: d.Config.MailConnections.GoogleClientSecret},
	})
	m.Interviews = interviews.New(d.Pool, interviewApplications{m.Pipeline}, m.Identity, interviewMailer{m.Identity, m.Emails},
		interviews.Settings{TimeZone: d.Config.TimeZone, PublicURL: d.Config.PublicURL})
	m.Placements = placements.New(d.Pool, placementApplications{m.Pipeline, m.Jobs}, m.Identity, placements.Invoicing{
		Terms: placementTerms{m.Companies}, Contacts: placementContacts{m.Contacts}, Mailer: placementMailer{m.Emails},
		Tenants: m.Identity, TimeZone: d.Config.TimeZone,
	})
	m.Reports = reports.New(d.Pool, reports.Deps{
		Applications: reportApplications{m.Pipeline}, Sources: reportSources{m.Candidates}, Jobs: reportJobs{m.Jobs},
		Placements: reportPlacements{m.Placements}, Users: m.Identity,
	}, d.Config.TimeZone)
	m.Offers = offers.New(d.Pool, offerApplications{m.Pipeline}, m.Identity)
	m.Integrations = integrations.New(d.Pool, m.Identity, d.Config.AllowPrivateNetworks)
	m.Leads = leads.New(d.Pool, m.Companies, d.Config.AllowPrivateNetworks)
	m.Cvformats = cvformats.New(d.Pool, cvformats.Deps{
		Candidates: cvCandidates{m.Candidates}, Applications: cvApplications{m.Pipeline}, Documents: m.Documents,
		Tenants: m.Identity, Users: m.Identity, SourceCVs: cvSources{m.Documents},
		AI: cvReadingAI{
			all: aiUse{func() []tokensSince { return []tokensSince{m.Assistant, m.Matching, m.Documents, m.Cvformats, m.Jobs} }},
			ai:  m.AIProviders, cfg: d.Config.AI, tz: d.Config.TimeZone, now: time.Now,
		},
	}, d.Files, d.Config.MaxUploadBytes)
	m.Assistant = assistant.New(d.Pool, assistant.Deps{
		Candidates: assistantCandidates{m.Candidates}, CVs: m.Documents, Jobs: assistantJobs{m.Jobs},
		Applications: assistantApplications{m.Pipeline},
		Market:       assistantMarket{m},
		AI:           m.AIProviders,
		OtherAIUse:   aiUse{func() []tokensSince { return []tokensSince{m.Matching, m.Documents, m.Cvformats, m.Jobs} }},
	}, assistant.Settings{MonthlyTokens: d.Config.AI.MonthlyTokens, TimeZone: d.Config.TimeZone})
	m.Matching = matching.New(d.Pool, matching.Deps{
		Applications: matchingApplications{m.Pipeline}, Candidates: matchingCandidates{m.Candidates}, CVs: matchingCVs{m.Documents},
		Jobs: matchingJobs{m.Jobs}, Consent: m.Privacy, AI: m.AIProviders,
		OtherAIUse: aiUse{func() []tokensSince { return []tokensSince{m.Assistant, m.Documents, m.Cvformats, m.Jobs} }},
	}, matching.Settings{MonthlyTokens: d.Config.AI.MonthlyTokens, TimeZone: d.Config.TimeZone})
	m.Intake = intake.New(d.Pool, intake.Deps{
		Candidates: intakeCandidates{m.Candidates}, Documents: intakeDocuments{m.Documents}, Sources: m.Settings,
	}, d.Config.MaxUploadBytes)
	m.Documents.WhenReadDone(m.Intake.ReadDone) // imported CVs fill their candidates in
	m.Market = market.New(d.Pool, market.Deps{
		Jobs: marketJobs{m.Jobs}, Candidates: marketCandidates{m.Candidates}, Insights: marketInsights{m.Assistant},
	})
	m.Scheduling = scheduling.New(d.Pool, scheduling.Deps{
		Candidates: schedulingCandidates{m.Candidates}, Applications: schedulingApplications{m.Pipeline},
		Users: schedulingUsers{m.Identity}, Interviews: m.Interviews, Mailer: schedulingMailer{m.Emails},
	}, scheduling.Config{
		PublicURL: d.Config.PublicURL, OrganizerEmail: d.Config.SMTP.From, DefaultTimeZone: d.Config.TimeZone,
		AllowPrivateNetworks: d.Config.AllowPrivateNetworks,
	})
	m.Industries.UseRecords(industryRecords{m}, m.Settings)
	m.Candidates.UseIndustries(m.Industries)
	m.Companies.UseIndustries(m.Industries)
	m.Documents.UseIndustryRecognition(m.Industries.EnrichParsed)
	m.Settings.UseIndustries(m.Industries)
	m.Settings.CountUsesWith(optionUses{m.Contacts, m.Companies})
	m.Automations = automations.New(d.Pool, automations.Deps{
		Applications: automationApplications{m.Pipeline, m.Jobs}, Interviews: automationInterviews{m.Interviews},
		Placements: automationPlacements{m.Placements}, Companies: m.Companies, Users: m.Identity,
		Mailer: automationMailer{m.Emails}, TimeZone: d.Config.TimeZone,
	})
	// scaffold:constructors
	// A merged duplicate's CVs, jobs, notes, emails, consent and requests follow it.
	m.Candidates.MoveRecordsWith(m.Industries, m.Documents, m.Pipeline, m.Notes, m.Emails, m.Privacy)
	m.Candidates.WriteNotesWith(m.Notes)
	matchingHooks(m)
	m.Documents.UseAI(cvReadingAI{
		all: aiUse{func() []tokensSince { return []tokensSince{m.Assistant, m.Matching, m.Documents, m.Cvformats, m.Jobs} }},
		ai:  m.AIProviders, cfg: d.Config.AI, tz: d.Config.TimeZone, now: time.Now,
	})
	m.Jobs.UseAI(cvReadingAI{ // blind ad rewrites, in the same monthly allowance
		all: aiUse{func() []tokensSince { return []tokensSince{m.Assistant, m.Matching, m.Documents, m.Cvformats, m.Jobs} }},
		ai:  m.AIProviders, cfg: d.Config.AI, tz: d.Config.TimeZone, now: time.Now,
	})
	return m
}

// Each module's handler type is called HTTP; these aliases give the embedded
// fields distinct names.
type (
	identityHTTP     = identity.HTTP
	candidatesHTTP   = candidates.HTTP
	documentsHTTP    = documents.HTTP
	companiesHTTP    = companies.HTTP
	contactsHTTP     = contacts.HTTP
	jobsHTTP         = jobs.HTTP
	pipelineHTTP     = pipeline.HTTP
	notesHTTP        = notes.HTTP
	careersHTTP      = careers.HTTP
	interviewsHTTP   = interviews.HTTP
	emailsHTTP       = emails.HTTP
	placementsHTTP   = placements.HTTP
	reportsHTTP      = reports.HTTP
	offersHTTP       = offers.HTTP
	integrationsHTTP = integrations.HTTP
	leadsHTTP        = leads.HTTP
	cvformatsHTTP    = cvformats.HTTP
	assistantHTTP    = assistant.HTTP
	matchingHTTP     = matching.HTTP
	privacyHTTP      = privacy.HTTP
	settingsHTTP     = settings.HTTP
	intakeHTTP       = intake.HTTP
	marketHTTP       = market.HTTP
	aiprovidersHTTP  = aiproviders.HTTP
	schedulingHTTP   = scheduling.HTTP
	industriesHTTP   = industries.HTTP
	automationsHTTP  = automations.HTTP
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
	*reportsHTTP
	*offersHTTP
	*integrationsHTTP
	*leadsHTTP
	*cvformatsHTTP
	*assistantHTTP
	*matchingHTTP
	*privacyHTTP
	*settingsHTTP
	*intakeHTTP
	*marketHTTP
	*aiprovidersHTTP
	*schedulingHTTP
	*industriesHTTP
	*automationsHTTP
	// scaffold:embeds
}

var _ api.StrictServerInterface = (*Server)(nil)

// Server returns the composed HTTP handlers.
func (m *Modules) Server() *Server {
	return &Server{
		identityHTTP:     m.Identity.HTTP,
		candidatesHTTP:   m.Candidates.HTTP,
		documentsHTTP:    m.Documents.HTTP,
		companiesHTTP:    m.Companies.HTTP,
		contactsHTTP:     m.Contacts.HTTP,
		jobsHTTP:         m.Jobs.HTTP,
		pipelineHTTP:     m.Pipeline.HTTP,
		notesHTTP:        m.Notes.HTTP,
		careersHTTP:      m.Careers.HTTP,
		interviewsHTTP:   m.Interviews.HTTP,
		emailsHTTP:       m.Emails.HTTP,
		placementsHTTP:   m.Placements.HTTP,
		reportsHTTP:      m.Reports.HTTP,
		offersHTTP:       m.Offers.HTTP,
		integrationsHTTP: m.Integrations.HTTP,
		leadsHTTP:        m.Leads.HTTP,
		cvformatsHTTP:    m.Cvformats.HTTP,
		assistantHTTP:    m.Assistant.HTTP,
		matchingHTTP:     m.Matching.HTTP,
		privacyHTTP:      m.Privacy.HTTP,
		settingsHTTP:     m.Settings.HTTP,
		intakeHTTP:       m.Intake.HTTP,
		marketHTTP:       m.Market.HTTP,
		aiprovidersHTTP:  m.AIProviders.HTTP,
		schedulingHTTP:   m.Scheduling.HTTP,
		industriesHTTP:   m.Industries.HTTP,
		automationsHTTP:  m.Automations.HTTP,
		// scaffold:server-fields
	}
}

// JobHandlers merges every module's handlers for the core queue.
func (m *Modules) JobHandlers() (jobqueue.Handlers, error) {
	all := jobqueue.Handlers{}
	for _, handlers := range []jobqueue.Handlers{
		m.Documents.JobHandlers(),
		m.Cvformats.JobHandlers(),
		m.Jobs.JobHandlers(),
		m.Emails.JobHandlers(),
		m.Interviews.JobHandlers(),
		m.Integrations.JobHandlers(),
		m.Leads.JobHandlers(),
		m.Companies.JobHandlers(),
		m.Placements.JobHandlers(),
		m.Automations.JobHandlers(),
		m.Contacts.JobHandlers(),
		m.Assistant.JobHandlers(),
		m.Matching.JobHandlers(),
		m.AIProviders.JobHandlers(),
		m.Scheduling.JobHandlers(),
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
