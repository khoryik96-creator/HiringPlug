package jobs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the jobs use cases.
type Service struct {
	pool      *pgxpool.Pool
	companies Companies
	contacts  Contacts
	users     Users
	rules     FieldRules  // the company's mandatory fields
	files     *blob.Store // uploaded job descriptions
	ai        AI          // the company's AI, for blind ad rewrites (nil: off)
}

// Create adds a job; jdFileID (or "") is the job description its form was
// filled in from, kept with the job.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Details, jdFileID string) (View, error) {
	d, errs := Normalize(in)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.checkMandatory(ctx, tx, nil, d); err != nil {
			return err
		}
		if err := s.checkReferences(ctx, tx, d); err != nil {
			return err
		}
		row, err := store.New(tx).InsertJobOrder(ctx, store.InsertJobOrderParams{
			CompanyID: d.CompanyID, ContactID: d.ContactID, Title: d.Title, Status: d.Status, Location: d.Location,
			WorkType: d.WorkType, WorkArrangement: d.WorkArrangement, Openings: int32(d.Openings), //nolint:gosec // 1..999 per the contract
			Description: d.Description, OwnerID: ownerFor(p), Country: d.Country, IntakeDate: pgtype.Date{Time: d.IntakeDate, Valid: true},
		})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "job.created", EntityType: "job", EntityID: row.ID, Changes: audit.Diff(map[string]any{}, fields(d)),
		}); err != nil {
			return err
		}
		if jdFileID != "" {
			if err := linkJD(ctx, tx, jdFileID, row.ID); err != nil {
				return err
			}
		}
		out, err = s.view(ctx, tx, p, fromRow(row))
		return err
	})
	return out, err
}

// Get returns one job, with its salary budget if the user may see it.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getRow(ctx, store.New(tx), id)
		if err != nil {
			return err
		}
		out, err = s.view(ctx, tx, p, fromRow(row))
		return err
	})
	return out, err
}

// Update changes some fields; expectedVersion must be the version the user
// loaded. Nothing is written when nothing changes.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch Patch) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, id)
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("job").WithDetail("currentVersion", row.Version)
		}
		before := fromRow(row).Details
		after, errs := Normalize(patch.Apply(before))
		if err := errs.Err(); err != nil {
			return err
		}
		changes := audit.Diff(fields(before), fields(after))
		if len(changes) == 0 {
			out, err = s.view(ctx, tx, p, fromRow(row))
			return err
		}
		if err := s.checkMandatory(ctx, tx, &before, after); err != nil {
			return err
		}
		if err := s.checkReferences(ctx, tx, after); err != nil {
			return err
		}
		updated, err := q.UpdateJobOrder(ctx, store.UpdateJobOrderParams{
			ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small positive integers
			CompanyID: after.CompanyID, ContactID: after.ContactID, Title: after.Title, Status: after.Status,
			Location: after.Location, WorkType: after.WorkType, WorkArrangement: after.WorkArrangement,
			Openings: int32(after.Openings), Description: after.Description, //nolint:gosec // 1..999 per the contract
			Country: after.Country, IntakeDate: pgtype.Date{Time: after.IntakeDate, Valid: true},
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("job")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "job.updated", EntityType: "job", EntityID: id, Changes: changes}); err != nil {
			return err
		}
		if before.Status != after.Status {
			if err := closeAdsWithJob(ctx, tx, id, after.Status); err != nil {
				return err
			}
		}
		out, err = s.view(ctx, tx, p, fromRow(updated))
		return err
	})
	return out, err
}

// SetSalary replaces a job's salary budget (r nil clears it). expectedVersion
// is 0 when none exists yet, otherwise the version the user loaded.
func (s *Service) SetSalary(ctx context.Context, p authz.Principal, jobID string, expectedVersion int, r *SalaryRange) (*Salary, error) {
	if err := ValidateSalary(r).Err(); err != nil {
		return nil, err
	}
	var out *Salary
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := getRow(ctx, q, jobID); err != nil {
			return err
		}
		stale := apperr.StaleVersion("salary budget")
		version := int32(expectedVersion) //nolint:gosec // versions are small positive integers
		switch {
		case r == nil && expectedVersion == 0:
			return nil // nothing to clear
		case r == nil:
			n, err := q.DeleteJobSalary(ctx, store.DeleteJobSalaryParams{JobOrderID: jobID, ExpectedVersion: version})
			if err != nil {
				return err
			}
			if n == 0 {
				return stale
			}
		case expectedVersion == 0:
			row, err := q.InsertJobSalary(ctx, store.InsertJobSalaryParams{
				JobOrderID: jobID, MinAmount: r.Min, MaxAmount: r.Max, Currency: r.Currency, Period: r.Period,
			})
			if database.IsNoRows(err) {
				return stale // someone else added one first
			}
			if err != nil {
				return err
			}
			out = salaryFromRow(store.GetJobSalaryRow(row))
		default:
			row, err := q.UpdateJobSalary(ctx, store.UpdateJobSalaryParams{
				JobOrderID: jobID, ExpectedVersion: version, MinAmount: r.Min, MaxAmount: r.Max, Currency: r.Currency, Period: r.Period,
			})
			if database.IsNoRows(err) {
				return stale
			}
			if err != nil {
				return err
			}
			out = salaryFromRow(store.GetJobSalaryRow(row))
		}
		// Salary values never go into the audit log, only the fact they changed.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job.salary_updated", EntityType: "job", EntityID: jobID,
			Changes: map[string]any{"salary": audit.Redacted},
		})
	})
	return out, err
}

// Page is one page of a list, newest first.
type Page struct {
	Items      []Summary
	NextCursor *string
}

// List returns up to limit jobs, optionally matching query, status or company.
func (s *Service) List(ctx context.Context, p authz.Principal, query, status, companyID, contactID, cursor *string, limit int) (Page, error) {
	var page Page
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		// A word of the search may be the client's name rather than in the title.
		hits := []string{}
		if query != nil {
			var err error
			if hits, err = s.companies.WordHits(ctx, tx, *query); err != nil {
				return err
			}
		}
		rows, err := store.New(tx).ListJobOrders(ctx, store.ListJobOrdersParams{
			Cursor: cursor, Status: status, CompanyID: companyID, ContactID: contactID, Query: query, CompanyHits: hits, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
		})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1].ID
			page.NextCursor = &last
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.CompanyID
		}
		names, err := s.companies.Names(ctx, tx, ids)
		if err != nil {
			return err
		}
		page.Items = make([]Summary, len(rows))
		for i, r := range rows {
			page.Items[i] = Summary{
				ID: r.ID, Title: r.Title, CompanyID: r.CompanyID, CompanyName: names[r.CompanyID], Status: r.Status,
				Location: r.Location, Openings: int(r.Openings), UpdatedAt: r.UpdatedAt,
			}
		}
		return nil
	})
	return page, err
}

// Exists reports whether a job exists in the transaction's tenant.
func (s *Service) Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	return store.New(tx).JobOrderExists(ctx, id)
}

// Open returns the newest open jobs, at most limit.
func (s *Service) Open(ctx context.Context, tx pgx.Tx, limit int) ([]OpenJob, error) {
	rows, err := store.New(tx).OpenJobOrders(ctx, int32(limit)) //nolint:gosec // small constant
	out := make([]OpenJob, len(rows))
	for i, r := range rows {
		out[i] = OpenJob{ID: r.ID, Title: r.Title, CompanyID: r.CompanyID}
	}
	return out, err
}

// Briefs returns id -> title and company for the jobs that exist.
func (s *Service) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	rows, err := store.New(tx).JobOrderTitles(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Brief, len(rows))
	for _, r := range rows {
		out[r.ID] = Brief{Title: r.Title, CompanyID: r.CompanyID, ContactID: r.ContactID, WorkType: r.WorkType, OwnerID: r.OwnerID, CreatedAt: r.CreatedAt}
	}
	return out, nil
}

// checkReferences checks the company exists and the contact works there.
func (s *Service) checkReferences(ctx context.Context, tx pgx.Tx, d Details) error {
	var errs apperr.FieldErrors
	ok, err := s.companies.Exists(ctx, tx, d.CompanyID)
	if err != nil {
		return err
	}
	if !ok {
		errs.Add("companyId", "no such company")
	}
	if d.ContactID != nil {
		info, err := s.contacts.Info(ctx, tx, []string{*d.ContactID})
		if err != nil {
			return err
		}
		if c, found := info[*d.ContactID]; !found {
			errs.Add("contactId", "no such contact")
		} else if ok && c.CompanyID != d.CompanyID {
			errs.Add("contactId", "works at another company")
		}
	}
	return errs.Err()
}

// view adds names and, for users allowed to see it, the salary budget. The
// database applies the same rule (row-level security); checking here lets the
// API say "restricted" rather than "none recorded".
func (s *Service) view(ctx context.Context, tx pgx.Tx, p authz.Principal, j JobOrder) (View, error) {
	names, err := s.companies.Names(ctx, tx, []string{j.Details.CompanyID})
	if err != nil {
		return View{}, err
	}
	j.CompanyName = names[j.Details.CompanyID]
	if id := j.Details.ContactID; id != nil {
		info, err := s.contacts.Info(ctx, tx, []string{*id})
		if err != nil {
			return View{}, err
		}
		if c, ok := info[*id]; ok {
			j.ContactName = &c.Name
		}
	}
	if j.OwnerID != nil {
		names, err := s.users.UserNames(ctx, tx, []string{*j.OwnerID})
		if err != nil {
			return View{}, err
		}
		if n, ok := names[*j.OwnerID]; ok {
			j.OwnerName = &n
		}
	}
	v := View{JobOrder: j}
	if !p.Can(api.PermissionJobsSalaryRead) {
		v.SalaryRestricted = true
		return v, nil
	}
	row, err := store.New(tx).GetJobSalary(ctx, j.ID)
	if database.IsNoRows(err) {
		return v, nil
	}
	if err != nil {
		return View{}, err
	}
	v.Salary = salaryFromRow(row)
	return v, nil
}

func getRow(ctx context.Context, q *store.Queries, id string) (store.JobOrder, error) {
	row, err := q.GetJobOrder(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("job")
	}
	return row, err
}

func salaryFromRow(r store.GetJobSalaryRow) *Salary {
	rng := &SalaryRange{Min: r.MinAmount, Currency: r.Currency, Period: r.Period}
	if r.MaxAmount != "" {
		rng.Max = &r.MaxAmount
	}
	return &Salary{Version: int(r.Version), Range: rng, UpdatedAt: r.UpdatedAt}
}

func fromRow(r store.JobOrder) JobOrder {
	return JobOrder{
		ID: r.ID, Version: int(r.Version), OwnerID: r.OwnerID, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Details: Details{
			CompanyID: r.CompanyID, ContactID: r.ContactID, Title: r.Title, Status: r.Status, Location: r.Location,
			WorkType: r.WorkType, WorkArrangement: r.WorkArrangement, Openings: int(r.Openings), Description: r.Description,
			Country: r.Country, IntakeDate: r.IntakeDate.Time,
		},
	}
}

// fields lists the values the audit log compares.
func fields(d Details) map[string]any {
	return map[string]any{
		"companyId": d.CompanyID, "contactId": d.ContactID, "title": d.Title, "status": d.Status,
		"location": d.Location, "workType": d.WorkType, "workArrangement": d.WorkArrangement,
		"openings": d.Openings, "description": d.Description, "country": d.Country,
		"intakeDate": d.IntakeDate.Format(time.DateOnly),
	}
}
