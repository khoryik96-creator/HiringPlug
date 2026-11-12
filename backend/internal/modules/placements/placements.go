// Package placements records candidates placed in jobs: the start date, the
// guarantee period (leaving within it is a fall-through), the recruiter
// credited, whether the fee is invoiced and paid, and the agreed salary and
// fee.
//
// Recording a placement moves the application to placed, in the same
// transaction. Salary and fee live in their own table: without
// placements.fees:read the database returns no rows for it (row-level
// security), and without placements.fees:update it refuses writes.
//
// Layout (the same in every module):
//
//	placements.go  the module's public API and what it needs from other modules
//	domain.go      the rules, as pure functions on plain values (unit tested)
//	service.go     use cases: one transaction each
//	invoice*.go    invoicing: the client's terms, the recipient and due date,
//	               the invoice template, and the invoice emailed on the start date
//	http.go        HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/placements.sql); private to this module
package placements

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Application is what this module needs to know about an application.
type Application struct {
	Stage                      string
	CandidateID, CandidateName string
	JobID, JobTitle            string
	CompanyID, CompanyName     string
	ContactID                  *string // the job's hiring contact
	WorkType                   *string // the job's work type (full_time, contract, ...)
}

// ClientTerms are a client's terms of business (companies module): their
// rates by annual salary, the limits in SalaryCurrency.
type ClientTerms struct {
	Tiers          []ClientTier
	SalaryCurrency string
	Set            bool
}

// ClientTier is one rate, for annual salaries from SalaryFrom (inclusive)
// up to SalaryBelow (exclusive); nil limits are open, other nils not set.
type ClientTier struct {
	SalaryFrom, SalaryBelow, FeePercent *string
	PaymentTermsDays, GuaranteeDays     *int
	Remarks                             *string
}

// Contact is someone at a client who may receive invoices (contacts module).
type Contact struct {
	Name, CompanyID string
	Email           *string
}

// InvoiceMail is an invoice emailed to a client (emails module): from the
// agency, the invoice attached as a PDF, shown only to those who may see money.
type InvoiceMail struct {
	To, Subject, Body, From string
	FileName                string
	FileKey                 string // the same for the same invoice: a retry rewrites it
	PDF                     []byte
	ApplicationID           string
}

// Invoicing is what invoicing needs from other modules, each read in the
// caller's transaction, and the agency's time zone (invoices go out on the
// start date there).
type Invoicing struct {
	Terms interface {
		Terms(ctx context.Context, tx pgx.Tx, companyID string) (ClientTerms, error)
	}
	Contacts interface {
		Contacts(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Contact, error)
	}
	Mailer interface {
		SendInvoice(ctx context.Context, tx pgx.Tx, tenantID string, m InvoiceMail) error
	}
	Tenants interface {
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	TimeZone *time.Location
}

// Applications reads applications and moves them to placed (pipeline module).
type Applications interface {
	Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	// PlacedOfCompany returns the applications on a company's jobs that are
	// or were ever placed (only they can have a placement: its billing).
	PlacedOfCompany(ctx context.Context, tx pgx.Tx, companyID string) ([]string, error)
	// PlacedOnJob is the same for one job (the placement its retainer is taken off).
	PlacedOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]string, error)
	// JobExists reports whether the job exists (for its retainer).
	JobExists(ctx context.Context, tx pgx.Tx, jobID string) (bool, error)
	Place(ctx context.Context, tx pgx.Tx, id string) error
}

// Users checks and names the recruiters credited (identity module).
type Users interface {
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the placements module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, applications Applications, users Users, invoicing Invoicing) *Module {
	if invoicing.TimeZone == nil {
		invoicing.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, applications: applications, users: users, inv: invoicing}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindPlacementInvoice: m.service.invoiceJob}
}

// Fact is a placement as reports count it. FeeAmount is nil when none is
// recorded or the user may not see fees (the database leaves them out).
type Fact struct {
	RecruiterID            *string
	Status                 string
	FeeAmount, FeeCurrency *string
}

// Facts returns the placements starting between two days (inclusive), in the
// caller's transaction.
func (m *Module) Facts(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]Fact, error) {
	rows, err := store.New(tx).PlacementFacts(ctx, store.PlacementFactsParams{FromDay: fromDay, ToDay: toDay})
	if err != nil {
		return nil, err
	}
	out := make([]Fact, len(rows))
	for i, r := range rows {
		fee, err := database.DecimalString(r.FeeAmount)
		if err != nil {
			return nil, err
		}
		out[i] = Fact{RecruiterID: r.RecruiterID, Status: r.Status, FeeAmount: fee, FeeCurrency: r.FeeCurrency}
	}
	return out, nil
}
