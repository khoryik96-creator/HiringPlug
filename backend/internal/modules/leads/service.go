package leads

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the lead use cases.
type Service struct {
	pool         *pgxpool.Pool
	companies    Companies
	allowPrivate bool
}

const oneSourcePerAddress = "lead_source_tenant_id_kind_identifier_key"

// now is to the second, so a check's time survives the trip through the job
// payload and the database unchanged.
func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

func (s *Service) checkCompany(ctx context.Context, tx pgx.Tx, id *string) error {
	if id == nil {
		return nil
	}
	ok, err := s.companies.Exists(ctx, tx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation(apperr.FieldErrors{{Field: "companyId", Message: "is not one of your companies"}})
	}
	return nil
}

// queueCheck schedules a source's check for at (replacing any other: a
// check finds the source scheduled for another time and does nothing).
func queueCheck(ctx context.Context, tx pgx.Tx, tenantID, id string, at time.Time) error {
	_, err := jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindLeadSourceCheck, api.LeadSourceCheckJob{SourceId: id, RunAt: at}, at)
	return err
}

// Sources returns the company's lead sources.
func (s *Service) Sources(ctx context.Context, p authz.Principal) ([]store.LeadSource, error) {
	var out []store.LeadSource
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListSources(ctx)
		return err
	})
	return out, err
}

// CreateSource adds a source and queues its first check.
func (s *Service) CreateSource(ctx context.Context, p authz.Principal, kind, identifier, name string, companyID *string) (store.LeadSource, error) {
	identifier, name, errs := NormalizeSource(kind, identifier, name)
	if err := errs.Err(); err != nil {
		return store.LeadSource{}, err
	}
	var out store.LeadSource
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.checkCompany(ctx, tx, companyID); err != nil {
			return err
		}
		at := now()
		row, err := store.New(tx).InsertSource(ctx, store.InsertSourceParams{Kind: kind, Identifier: identifier, Name: name, CompanyID: companyID, NextCheckAt: &at})
		if database.IsUniqueViolation(err, oneSourcePerAddress) {
			return apperr.Validation(apperr.FieldErrors{{Field: "identifier", Message: "is already a source"}})
		}
		if err != nil {
			return err
		}
		out = row
		if err := queueCheck(ctx, tx, p.TenantID, row.ID, at); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "lead_source.created", EntityType: "lead_source", EntityID: row.ID, Changes: map[string]any{"kind": kind, "identifier": identifier}})
	})
	return out, err
}

// SourcePatch is a partial change of a source.
type SourcePatch struct {
	Name      apiconv.Change[string]
	CompanyID apiconv.Change[*string]
	Active    apiconv.Change[bool]
}

// UpdateSource renames a source, links it to a company, or turns its checks
// off or on (back on: checked straight away).
func (s *Service) UpdateSource(ctx context.Context, p authz.Principal, id string, version int, patch SourcePatch) (store.LeadSource, error) {
	var out store.LeadSource
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetSource(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("lead source")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != version {
			return apperr.StaleVersion("lead source")
		}
		name, companyID, active, next := row.Name, row.CompanyID, row.Active, row.NextCheckAt
		apiconv.Apply(&name, patch.Name)
		apiconv.Apply(&companyID, patch.CompanyID)
		apiconv.Apply(&active, patch.Active)
		if name = strings.Join(strings.Fields(name), " "); name == "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "is required"}})
		}
		if patch.CompanyID.Set {
			if err := s.checkCompany(ctx, tx, companyID); err != nil {
				return err
			}
		}
		switch {
		case !active:
			next = nil
		case !row.Active: // turned back on
			at := now()
			next = &at
			if err := queueCheck(ctx, tx, p.TenantID, id, at); err != nil {
				return err
			}
		}
		out, err = q.UpdateSource(ctx, store.UpdateSourceParams{ID: id, ExpectedVersion: int32(version), Name: name, CompanyID: companyID, Active: active, NextCheckAt: next}) //nolint:gosec // versions are small
		if database.IsNoRows(err) {
			return apperr.StaleVersion("lead source")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "lead_source.updated", EntityType: "lead_source", EntityID: id,
			Changes: audit.Diff(map[string]any{"name": row.Name, "companyId": row.CompanyID, "active": row.Active}, map[string]any{"name": name, "companyId": companyID, "active": active})})
	})
	return out, err
}

// DeleteSource removes a source; its leads stay.
func (s *Service) DeleteSource(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		n, err := store.New(tx).DeleteSource(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("lead source")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "lead_source.deleted", EntityType: "lead_source", EntityID: id})
	})
}

// CheckNow queues a check of an active source now.
func (s *Service) CheckNow(ctx context.Context, p authz.Principal, id string) (store.LeadSource, error) {
	var out store.LeadSource
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetSource(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("lead source")
		}
		if err != nil {
			return err
		}
		if !row.Active {
			return apperr.Validation(apperr.FieldErrors{{Field: "active", Message: "turn the source on first"}})
		}
		at := now()
		if out, err = q.ScheduleCheck(ctx, store.ScheduleCheckParams{ID: id, NextCheckAt: &at}); err != nil {
			return err
		}
		return queueCheck(ctx, tx, p.TenantID, id, at)
	})
	return out, err
}

// LeadFilter narrows the list of leads.
type LeadFilter struct {
	Status, SourceID, Q *string
	Open                *bool
}

// Leads returns leads, newest first.
func (s *Service) Leads(ctx context.Context, p authz.Principal, f LeadFilter) ([]store.JobLead, error) {
	var out []store.JobLead
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListLeads(ctx, store.ListLeadsParams{Status: f.Status, SourceID: f.SourceID, Open: f.Open, Q: f.Q})
		return err
	})
	return out, err
}

// ManualLead is a job ad saved by hand.
type ManualLead struct {
	CompanyName, Title   string
	Location, URL, Notes *string
	CompanyID            *string
}

func trimmed(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	t := strings.TrimSpace(*s)
	return &t
}

// CreateLead saves a job ad found by hand.
func (s *Service) CreateLead(ctx context.Context, p authz.Principal, in ManualLead) (store.JobLead, error) {
	var errs apperr.FieldErrors
	in.CompanyName, in.Title = strings.Join(strings.Fields(in.CompanyName), " "), strings.Join(strings.Fields(in.Title), " ")
	in.Location, in.URL, in.Notes = trimmed(in.Location), trimmed(in.URL), trimmed(in.Notes)
	if in.CompanyName == "" {
		errs.Add("companyName", "is required")
	}
	if in.Title == "" {
		errs.Add("title", "is required")
	}
	if in.URL != nil {
		if u, err := url.Parse(*in.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			errs.Add("url", "must be the ad's address, e.g. https://...")
		}
	}
	if err := errs.Err(); err != nil {
		return store.JobLead{}, err
	}
	var out store.JobLead
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.checkCompany(ctx, tx, in.CompanyID); err != nil {
			return err
		}
		row, err := store.New(tx).InsertLead(ctx, store.InsertLeadParams{
			CompanyName: in.CompanyName, CompanyID: in.CompanyID, Title: in.Title, Location: in.Location, Url: in.URL, Notes: in.Notes,
		})
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{Action: "job_lead.created", EntityType: "job_lead", EntityID: row.ID, Changes: map[string]any{"companyName": in.CompanyName, "title": in.Title}})
	})
	return out, err
}

// LeadPatch is a partial change of a lead.
type LeadPatch struct {
	Status    apiconv.Change[string]
	Notes     apiconv.Change[*string]
	CompanyID apiconv.Change[*string]
}

// UpdateLead changes a lead's status, notes or company.
func (s *Service) UpdateLead(ctx context.Context, p authz.Principal, id string, version int, patch LeadPatch) (store.JobLead, error) {
	var out store.JobLead
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetLead(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("lead")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != version {
			return apperr.StaleVersion("lead")
		}
		status, notes, companyID := row.Status, row.Notes, row.CompanyID
		apiconv.Apply(&status, patch.Status)
		apiconv.Apply(&notes, patch.Notes)
		apiconv.Apply(&companyID, patch.CompanyID)
		notes = trimmed(notes)
		if patch.CompanyID.Set {
			if err := s.checkCompany(ctx, tx, companyID); err != nil {
				return err
			}
		}
		out, err = q.UpdateLead(ctx, store.UpdateLeadParams{ID: id, ExpectedVersion: int32(version), Status: status, Notes: notes, CompanyID: companyID}) //nolint:gosec // versions are small
		if database.IsNoRows(err) {
			return apperr.StaleVersion("lead")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "job_lead.updated", EntityType: "job_lead", EntityID: id,
			Changes: audit.Diff(map[string]any{"status": row.Status, "notes": row.Notes, "companyId": row.CompanyID}, map[string]any{"status": status, "notes": notes, "companyId": companyID})})
	})
	return out, err
}
