package app

// This file is the one place modules are wired together. To add a module
// (`task new:module -- name` does all of this for you):
//  1. add a field to Modules and construct it in NewModules;
//  2. embed its HTTP handlers in Server (if it has endpoints);
//  3. add its JobHandlers in JobHandlers (if it handles jobs).
// The compiler then checks that every endpoint in the contract has a handler,
// and the worker refuses to start if a job kind has none.

import (
	"fmt"
	"maps"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/careers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/folders"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/intake"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/market"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notifications"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/reports"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/shortlists"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	// scaffold:imports
)

// Modules holds every module, wired together.
type Modules struct {
	Identity      *identity.Module
	Candidates    *candidates.Module
	Documents     *documents.Module
	Companies     *companies.Module
	Contacts      *contacts.Module
	Jobs          *jobs.Module
	Pipeline      *pipeline.Module
	Notes         *notes.Module
	Careers       *careers.Module
	Interviews    *interviews.Module
	Emails        *emails.Module
	Placements    *placements.Module
	Reports       *reports.Module
	Offers        *offers.Module
	Integrations  *integrations.Module
	Leads         *leads.Module
	Cvformats     *cvformats.Module
	Assistant     *assistant.Module
	Matching      *matching.Module
	Meaning       *meaning.Module
	Privacy       *privacy.Module
	Settings      *settings.Module
	Intake        *intake.Module
	Market        *market.Module
	AIProviders   *aiproviders.Module
	AIUsage       *aiusage.Module
	Scheduling    *scheduling.Module
	Industries    *industries.Module
	Automations   *automations.Module
	Folders       *folders.Module
	Bin           *bin.Module
	Calendars     *calendars.Module
	Claims        *claims.Module
	InterviewPrep *interviewprep.Module
	Notifications *notifications.Module
	Shortlists    *shortlists.Module
	// scaffold:fields
}

// NewModules builds every module. A module may receive another module (or a
// small interface it satisfies) to call its public API, as Documents does
// with Candidates.
func NewModules(d Deps) *Modules {
	m := &Modules{}
	m.Identity = identity.New(d.Pool, d.Verifier, identityMailer{m}, d.Config.PublicURL)
	m.Identity.UseTwoStepKey(d.Config.AuthJWTSecret)
	m.Settings = settings.New(d.Pool)
	m.Settings.UseBackups(settings.BackupSetup{Dir: d.Config.BackupDir, OperatorTenant: d.Config.BackupTenant})
	m.Industries = industries.New(d.Pool)
	m.AIProviders = aiproviders.New(d.Pool, aiproviders.Settings{ServerOn: d.Config.AI.Enabled, KeysSecret: d.Config.AI.KeysSecret})
	// The AI use ledger and each company's monthly AI limit (the server's as default).
	m.AIUsage = aiusage.New(d.Pool, aiusage.Deps{Users: m.Identity},
		aiusage.Config{DefaultTokens: d.Config.AI.MonthlyTokens, TimeZone: d.Config.TimeZone})
	m.AIProviders.RecordUseWith(aiLedger{m.AIUsage}) // a key check uses the AI too
	m.Candidates = candidates.New(d.Pool, d.Config.DefaultPhoneArea, m.Identity, m.Settings)
	m.Documents = documents.New(d.Pool, d.Files, documentCandidates{m.Candidates}, d.Config.MaxUploadBytes)
	m.Candidates.SearchCVsWith(m.Documents) // advanced search looks in their CVs
	m.Companies = companies.New(d.Pool, d.Config.AllowPrivateNetworks, d.Files, d.Config.MaxUploadBytes, m.Identity)
	m.Contacts = contacts.New(d.Pool, m.Companies, m.Settings, d.MailServers, d.Mailboxes, d.Files, d.Config.MaxUploadBytes)
	m.Contacts.ConfigureApollo(d.Config.Apollo, nil)
	m.Contacts.CheckApolloUsersWith(m.Identity) // a paid lookup re-checks the recruiter's access
	m.Jobs = jobs.New(d.Pool, m.Companies, jobContacts{m.Contacts}, m.Identity, m.Settings, d.Files, d.Config.MaxUploadBytes)
	m.useAgencyTime(d.Config.TimeZone)
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
		PhoneRegion: d.Config.DefaultPhoneArea,
		Candidates:  emailCandidates{m.Candidates, m.Industries, m.Settings}, Applications: emailApplications{m.Pipeline, m.Jobs, m.Contacts},
		Interviews: emailInterviews{m}, Tenants: m.Identity, Consent: m.Privacy, Users: m.Identity,
		Documents: emailDocuments{m.Documents}, Policy: m.Settings, Contacts: emailContacts{m.Contacts, m.Companies},
		Undeliverable: emailUndeliverable{m.Contacts}, Bounces: m.Contacts, InterviewReminders: calendarReminderMail{m}, Invoices: invoiceDelivery{m},
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
	m.Pipeline.WhenUnplaced(m.Placements.BackOut) // pulled out of placed: the sheet stays, backed out
	m.Reports = reports.New(d.Pool, reports.Deps{
		Applications: reportApplications{m.Pipeline}, Sources: reportSources{m.Candidates}, Jobs: reportJobs{m.Jobs},
		Placements: reportPlacements{m.Placements}, Users: m.Identity,
		Activity: reportActivity{candidates: m.Candidates, notes: m.Notes, pipeline: m.Pipeline, emails: m.Emails,
			interviews: m.Interviews, placements: m.Placements, identity: m.Identity},
		Forecast: reportForecast{pipeline: m.Pipeline, placements: m.Placements, jobs: m.Jobs, companies: m.Companies},
		Labels:   m.Settings,
	}, d.Config.TimeZone)
	m.Offers = offers.New(d.Pool, offerApplications{m.Pipeline}, m.Identity)
	m.Integrations = integrations.New(d.Pool, m.Identity, d.Config.AllowPrivateNetworks, d.Config.WebhookKeysSecret, d.Config.AuthJWTSecret)
	m.Leads = leads.New(d.Pool, m.Companies, d.Config.AllowPrivateNetworks)
	m.Cvformats = cvformats.New(d.Pool, cvformats.Deps{
		Candidates: cvCandidates{m.Candidates}, Applications: cvApplications{m.Pipeline}, Documents: m.Documents,
		Tenants: m.Identity, Users: m.Identity, SourceCVs: cvSources{m.Documents}, Files: cvFiles{m.Documents},
		AI: cvReadingAI{aiLedger{m.AIUsage}, m.AIProviders},
	}, d.Files, d.Config.MaxUploadBytes)
	m.Assistant = assistant.New(d.Pool, assistant.Deps{
		Candidates: assistantCandidates{m.Candidates}, CVs: m.Documents, Jobs: assistantJobs{m.Jobs},
		Applications: assistantApplications{m.Pipeline},
		Market:       assistantMarket{m},
		AI:           m.AIProviders,
		Agency:       m.Identity,
		Usage:        assistantUsage{aiLedger{m.AIUsage}},
	})
	// Search by meaning: candidates read by the AI worker's meaning model, ranked here.
	m.Meaning = meaning.New(d.Pool, meaning.Deps{Candidates: meaningCandidates{m.Candidates}, CVs: meaningCVs{m.Documents}})
	m.Candidates.SearchByMeaningWith(candidatesMeaning{m.Meaning})
	m.Matching = matching.New(d.Pool, matching.Deps{
		Applications: matchingApplications{m.Pipeline}, Candidates: matchingCandidates{m.Candidates}, CVs: matchingCVs{m.Documents},
		Jobs: matchingJobs{m.Jobs}, Consent: m.Privacy, AI: m.AIProviders, Meaning: matchingMeaning{m.Meaning},
		Usage: aiLedger{m.AIUsage},
	})
	m.Intake = intake.New(d.Pool, intake.Deps{
		Candidates: intakeCandidates{m.Candidates}, Documents: intakeDocuments{m.Documents}, Sources: m.Settings,
	}, d.Config.MaxUploadBytes)
	// Imported CVs fill their candidates in; every CV read suggests a work authorization.
	// Last: search by meaning reads the candidate as the CV left them.
	m.Documents.WhenReadDone(m.Intake.ReadDone, workAuthFromCV{m.Documents, m.Candidates}.ReadDone,
		profileFromNewestCV{m.Documents, m.Candidates}.ReadDone, m.Meaning.ReadDone)
	m.Documents.WhenCVBinned(m.Meaning.CVBinned)
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
	m.Companies.UseIndustries(companyIndustries{m.Industries})
	m.Documents.UseIndustryRecognition(m.Industries.EnrichParsed)
	m.Documents.UseIndustryLabels(m.Industries.Labels)
	m.Settings.UseIndustries(m.Industries)
	m.Automations = automations.New(d.Pool, automations.Deps{
		Applications: automationApplications{m.Pipeline, m.Jobs}, Interviews: automationInterviews{m.Interviews},
		Placements: automationPlacements{m.Placements}, Companies: m.Companies, Users: m.Identity,
		Mailer: automationMailer{m.Emails}, TimeZone: d.Config.TimeZone,
	})
	m.Bin = bin.New(d.Pool, binDeps(m, d.Files.Delete))
	m.Documents.BinWith(m.Bin.RecordDocument)
	m.Folders = folders.New(d.Pool, folderUsers{m.Identity}, folderRecords{m.Candidates, m.Contacts, m.Companies})
	// The lists show a folder's records.
	m.Candidates.UseFolders(m.Folders.ItemsOf(folders.Candidates))
	m.Contacts.UseFolders(m.Folders.ItemsOf(folders.Contacts))
	m.Companies.UseFolders(m.Folders.ItemsOf(folders.Companies))
	m.Calendars = calendars.New(d.Pool)
	m.Calendars.ConnectOutlookWith(calendars.Deps{
		Apps: calendarApps{m.Emails}, Sources: calendarSources{m.Interviews, m.Scheduling}, Users: calendarUsers{m.Identity},
		Recipients:   calendarRecipients{m.Pipeline, m.Candidates, m.Privacy},
		ReminderMail: calendarReminderMail{m}, Interviews: calendarInterviews{m.Interviews},
	}, calendars.Config{KeysSecret: d.Config.MailConnections.KeysSecret, PublicURL: d.Config.PublicURL, TimeZone: d.Config.TimeZone})
	m.Interviews.UseCalendar(m.Calendars)
	m.Scheduling.UseCalendar(m.Calendars)
	m.Claims = claims.New(d.Pool, d.Files, claims.Deps{
		Users: m.Identity, Options: claimOptions{m.Settings}, Records: claimRecords{m.Candidates, m.Contacts, m.Companies},
	}, d.Config.MaxUploadBytes, d.Config.TimeZone)
	m.Claims.UseStorage(claimStorageApps{m.Emails}, claims.StorageConfig{
		KeysSecret: d.Config.MailConnections.KeysSecret, PublicURL: d.Config.PublicURL,
		Dropbox: claims.App{ClientID: d.Config.Dropbox.AppKey, ClientSecret: d.Config.Dropbox.AppSecret},
	})
	m.Settings.CountUsesWith(optionUses{m.Contacts, m.Companies, m.Claims}) // records holding an option keep it
	m.InterviewPrep = interviewprep.New(d.Pool, interviewprep.Deps{
		Jobs: interviewPrepJobs{m.Jobs}, Companies: interviewPrepCompanies{m.Companies},
		AI: cvReadingAI{aiLedger{m.AIUsage}, m.AIProviders}, // prep material, in the same monthly allowance
	}, d.Config.TimeZone)
	m.Notifications = notifications.New(d.Pool, notificationSources{m}, m.Identity, notificationMailer{m}, notifications.Settings{TimeZone: d.Config.TimeZone, PublicURL: d.Config.PublicURL})
	m.Shortlists = newShortlists(d, m)
	// scaffold:constructors
	// A merged duplicate's CVs, jobs, notes, emails, consent and requests follow it.
	m.Candidates.MoveRecordsWith(m.Industries, m.Documents, m.Pipeline, m.Notes, m.Emails, m.Privacy, m.Scheduling, m.Cvformats, m.Intake, m.Folders, m.Claims, m.Shortlists)
	// A merged duplicate company's contacts, jobs, notes and the rest follow it;
	// a merged contact's emails, jobs, invoices and notes follow them.
	m.Companies.MoveRecordsWith(m.Contacts, m.Jobs, m.Notes, m.Folders, m.Leads, m.Automations, m.Industries)
	m.Contacts.MoveRecordsWith(m.Jobs, m.Notes, m.Folders, m.Emails, m.Placements)
	m.Pipeline.KeepRecordsWith(m.Interviews.OnApplications, m.Offers.OnApplications, m.Placements.OnApplications, m.Shortlists.OnApplications)
	m.Candidates.WriteNotesWith(m.Notes)
	matchingHooks(m)
	// CVs read, blind ad rewrites and a new company's industry: one monthly allowance.
	reading := cvReadingAI{aiLedger{m.AIUsage}, m.AIProviders}
	m.Documents.UseAI(reading)
	m.Jobs.UseAI(reading)
	m.Companies.UseAI(reading)
	return m
}

// Each module's handler type is called HTTP; these aliases give the embedded
// fields distinct names.
type (
	identityHTTP      = identity.HTTP
	candidatesHTTP    = candidates.HTTP
	documentsHTTP     = documents.HTTP
	companiesHTTP     = companies.HTTP
	contactsHTTP      = contacts.HTTP
	jobsHTTP          = jobs.HTTP
	pipelineHTTP      = pipeline.HTTP
	notesHTTP         = notes.HTTP
	careersHTTP       = careers.HTTP
	interviewsHTTP    = interviews.HTTP
	emailsHTTP        = emails.HTTP
	placementsHTTP    = placements.HTTP
	reportsHTTP       = reports.HTTP
	offersHTTP        = offers.HTTP
	integrationsHTTP  = integrations.HTTP
	leadsHTTP         = leads.HTTP
	cvformatsHTTP     = cvformats.HTTP
	assistantHTTP     = assistant.HTTP
	matchingHTTP      = matching.HTTP
	meaningHTTP       = meaning.HTTP
	privacyHTTP       = privacy.HTTP
	settingsHTTP      = settings.HTTP
	intakeHTTP        = intake.HTTP
	marketHTTP        = market.HTTP
	aiprovidersHTTP   = aiproviders.HTTP
	aiusageHTTP       = aiusage.HTTP
	schedulingHTTP    = scheduling.HTTP
	industriesHTTP    = industries.HTTP
	automationsHTTP   = automations.HTTP
	foldersHTTP       = folders.HTTP
	binHTTP           = bin.HTTP
	calendarsHTTP     = calendars.HTTP
	claimsHTTP        = claims.HTTP
	interviewprepHTTP = interviewprep.HTTP
	notificationsHTTP = notifications.HTTP
	shortlistsHTTP    = shortlists.HTTP
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
	*meaningHTTP
	*privacyHTTP
	*settingsHTTP
	*intakeHTTP
	*marketHTTP
	*aiprovidersHTTP
	*aiusageHTTP
	*schedulingHTTP
	*industriesHTTP
	*automationsHTTP
	*foldersHTTP
	*binHTTP
	*calendarsHTTP
	*claimsHTTP
	*interviewprepHTTP
	*notificationsHTTP
	*shortlistsHTTP
	// scaffold:embeds
}

var _ api.StrictServerInterface = (*Server)(nil)

// Server returns the composed HTTP handlers.
func (m *Modules) Server() *Server {
	return &Server{
		identityHTTP:      m.Identity.HTTP,
		candidatesHTTP:    m.Candidates.HTTP,
		documentsHTTP:     m.Documents.HTTP,
		companiesHTTP:     m.Companies.HTTP,
		contactsHTTP:      m.Contacts.HTTP,
		jobsHTTP:          m.Jobs.HTTP,
		pipelineHTTP:      m.Pipeline.HTTP,
		notesHTTP:         m.Notes.HTTP,
		careersHTTP:       m.Careers.HTTP,
		interviewsHTTP:    m.Interviews.HTTP,
		emailsHTTP:        m.Emails.HTTP,
		placementsHTTP:    m.Placements.HTTP,
		reportsHTTP:       m.Reports.HTTP,
		offersHTTP:        m.Offers.HTTP,
		integrationsHTTP:  m.Integrations.HTTP,
		leadsHTTP:         m.Leads.HTTP,
		cvformatsHTTP:     m.Cvformats.HTTP,
		assistantHTTP:     m.Assistant.HTTP,
		matchingHTTP:      m.Matching.HTTP,
		meaningHTTP:       m.Meaning.HTTP,
		privacyHTTP:       m.Privacy.HTTP,
		settingsHTTP:      m.Settings.HTTP,
		intakeHTTP:        m.Intake.HTTP,
		marketHTTP:        m.Market.HTTP,
		aiprovidersHTTP:   m.AIProviders.HTTP,
		aiusageHTTP:       m.AIUsage.HTTP,
		schedulingHTTP:    m.Scheduling.HTTP,
		industriesHTTP:    m.Industries.HTTP,
		automationsHTTP:   m.Automations.HTTP,
		foldersHTTP:       m.Folders.HTTP,
		binHTTP:           m.Bin.HTTP,
		calendarsHTTP:     m.Calendars.HTTP,
		claimsHTTP:        m.Claims.HTTP,
		interviewprepHTTP: m.InterviewPrep.HTTP,
		notificationsHTTP: m.Notifications.HTTP,
		shortlistsHTTP:    m.Shortlists.HTTP,
		// scaffold:server-fields
	}
}

// OutsideJobHandlers are the core jobs that wait minutes on another service
// and so run with no queue transaction open (see jobqueue.OutsideHandlers).
func (m *Modules) OutsideJobHandlers() jobqueue.OutsideHandlers {
	all := jobqueue.OutsideHandlers{}
	for _, handlers := range []jobqueue.OutsideHandlers{m.Calendars.OutsideJobHandlers(), m.Integrations.OutsideJobHandlers(), m.Emails.OutsideJobHandlers(), m.Claims.OutsideJobHandlers(), m.Contacts.OutsideJobHandlers()} {
		maps.Copy(all, handlers)
	}
	return all
}

// DeadJobHandlers merges the modules' handlers for jobs given up.
func (m *Modules) DeadJobHandlers() jobqueue.Handlers {
	all := jobqueue.Handlers{}
	for _, handlers := range []jobqueue.Handlers{m.Emails.DeadJobHandlers(), m.Claims.DeadJobHandlers()} {
		maps.Copy(all, handlers)
	}
	return all
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
		m.Meaning.JobHandlers(),
		m.AIProviders.JobHandlers(),
		m.Scheduling.JobHandlers(),
		m.Calendars.JobHandlers(),
		m.Bin.JobHandlers(),
		m.Claims.JobHandlers(),
		m.InterviewPrep.JobHandlers(),
		m.Notifications.JobHandlers(),
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
