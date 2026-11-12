package companies

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const nameUniqueIndex = "company_tenant_name_uq"

// Service holds the companies use cases.
type Service struct {
	industries   IndustryCatalogue
	pool         *pgxpool.Pool
	allowPrivate bool        // company websites on private networks (local testing only)
	files        *blob.Store // documents kept with terms and benefits
	users        Users
}

// Create adds a company.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Details) (Company, error) {
	d, errs := Normalize(in)
	if err := errs.Err(); err != nil {
		return Company{}, err
	}
	var out Company
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		q := store.New(tx)
		if s.industries != nil {
			var e error
			d.Industry, e = s.industries.SummaryIn(ctx, tx, d.IndustrySelections, d.Industry)
			if e != nil {
				return e
			}
		}
		if err := checkNameFree(ctx, q, d.Name, ""); err != nil {
			return err
		}
		row, err := q.InsertCompany(ctx, store.InsertCompanyParams{
			Name: d.Name, Type: d.Type, Category: d.Category, Website: d.Website, LinkedinUrl: d.LinkedinURL, Industry: d.Industry, Size: d.Size,
			Phone: d.Phone, Address: d.Address, City: d.City, Country: d.Country,
			RegistrationNumber: d.RegistrationNumber, About: d.About,
		})
		if database.IsUniqueViolation(err, nameUniqueIndex) {
			return duplicateName("") // added at the same moment by someone else
		}
		if err != nil {
			return err
		}
		if err := s.saveIndustries(ctx, tx, row.ID, d); err != nil {
			return err
		}
		row, err = q.GetCompany(ctx, row.ID)
		if err != nil {
			return err
		}
		out = fromRow(row)
		if err := s.readIndustries(ctx, tx, &out); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "company.created", EntityType: "company", EntityID: row.ID,
			Changes: audit.Diff(map[string]any{}, fields(d)),
		})
	})
	return out, err
}

// Get returns one company.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (Company, error) {
	var out Company
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetCompany(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("company")
		}
		if err != nil {
			return err
		}
		out = fromRow(row)
		return s.readIndustries(ctx, tx, &out)
	})
	return out, err
}

// Update changes some fields. expectedVersion must be the version the user
// loaded, or the update is refused (no lost updates). Nothing is written when
// nothing changes.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch Patch) (Company, error) {
	var out Company
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		q := store.New(tx)
		row, err := q.GetCompany(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("company")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("company").WithDetail("currentVersion", row.Version)
		}
		if s.industries != nil && patch.IndustrySelections != nil {
			label, e := s.industries.SummaryIn(ctx, tx, patch.IndustrySelections, nil)
			if e != nil {
				return e
			}
			patch.Industry.Set = true
			patch.Industry.Value = label
		}
		out, err = update(ctx, tx, q, row, patch, nil)
		if err != nil {
			return err
		}
		if patch.IndustrySelections != nil && out.Version == expectedVersion {
			row, err = q.TouchCompany(ctx, store.TouchCompanyParams{ID: id, ExpectedVersion: int32(expectedVersion)}) //nolint:gosec // positive record version from the contract
			if err != nil {
				return err
			}
			out = fromRow(row)
		}
		if err := s.saveIndustries(ctx, tx, out.ID, patch.Apply(out.Details)); err != nil {
			return err
		}
		row, err = q.GetCompany(ctx, out.ID)
		if err != nil {
			return err
		}
		out = fromRow(row)
		return s.readIndustries(ctx, tx, &out)
	})
	return out, err
}

// update changes a company in the caller's transaction: the rules, the
// unique name, the audit entry (with extra changes, e.g. where a value came
// from).
func update(ctx context.Context, tx pgx.Tx, q *store.Queries, row store.Company, patch Patch, extra map[string]any) (Company, error) {
	id := row.ID
	before := fromRow(row).Details
	after, errs := Normalize(patch.Apply(before))
	if err := errs.Err(); err != nil {
		return Company{}, err
	}
	changes := audit.Diff(fields(before), fields(after))
	if len(changes) == 0 {
		return fromRow(row), nil
	}
	if err := checkNameFree(ctx, q, after.Name, id); err != nil {
		return Company{}, err
	}
	updated, err := q.UpdateCompany(ctx, store.UpdateCompanyParams{
		ID: id, ExpectedVersion: row.Version,
		Name: after.Name, Type: after.Type, Category: after.Category, Website: after.Website, LinkedinUrl: after.LinkedinURL, Industry: after.Industry, Size: after.Size,
		Phone: after.Phone, Address: after.Address, City: after.City, Country: after.Country,
		RegistrationNumber: after.RegistrationNumber, About: after.About,
	})
	switch {
	case database.IsNoRows(err):
		return Company{}, apperr.StaleVersion("company") // changed between our read and our write
	case database.IsUniqueViolation(err, nameUniqueIndex):
		return Company{}, duplicateName("")
	case err != nil:
		return Company{}, err
	}
	for k, v := range extra {
		changes[k] = v
	}
	return fromRow(updated), audit.Record(ctx, tx, audit.Entry{Action: "company.updated", EntityType: "company", EntityID: id, Changes: changes})
}

// Page is one page of a list, newest first.
type Page struct {
	Items      []Summary
	NextCursor *string
}

// List returns up to limit companies, optionally matching query and type,
// starting after cursor (the last ID of the previous page).
func (s *Service) List(ctx context.Context, p authz.Principal, query, companyType, category, cursor *string, limit int) (Page, error) {
	var page Page
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		rows, err := store.New(tx).ListCompanies(ctx, store.ListCompaniesParams{
			Cursor: cursor, Type: companyType, Category: category, Query: query, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
		})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1].ID
			page.NextCursor = &last
		}
		page.Items = make([]Summary, len(rows))
		for i, r := range rows {
			page.Items[i] = Summary{ID: r.ID, Name: r.Name, Type: r.Type, Category: r.Category, Industry: r.Industry, City: r.City, Website: r.Website, LinkedinURL: r.LinkedinUrl, UpdatedAt: r.UpdatedAt}
		}
		return nil
	})
	return page, err
}

// Exists reports whether a company exists in the transaction's tenant.
func (s *Service) Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	return store.New(tx).CompanyExists(ctx, id)
}

// Names returns id -> name for the companies that exist.
func (s *Service) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	rows, err := store.New(tx).CompanyNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

func checkNameFree(ctx context.Context, q *store.Queries, name, selfID string) error {
	existing, err := q.FindCompanyIDByName(ctx, name)
	if database.IsNoRows(err) || (err == nil && existing == selfID) {
		return nil
	}
	if err != nil {
		return err
	}
	return duplicateName(existing)
}

func duplicateName(existingID string) error {
	e := apperr.New(api.ErrorCodeCOMPANYDUPLICATENAME, "Another company already has this name.")
	if existingID != "" {
		e = e.WithDetail("existingCompanyId", existingID)
	}
	return e
}

func fromRow(r store.Company) Company {
	return Company{
		ID: r.ID, Version: int(r.Version), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Details: Details{
			Name: r.Name, Type: r.Type, Category: r.Category, Website: r.Website, LinkedinURL: r.LinkedinUrl, Industry: r.Industry, Size: r.Size,
			Phone: r.Phone, Address: r.Address, City: r.City, Country: r.Country,
			RegistrationNumber: r.RegistrationNumber, About: r.About,
		},
	}
}

// fields lists the values the audit log compares.
func fields(d Details) map[string]any {
	return map[string]any{
		"name": d.Name, "type": d.Type, "category": d.Category, "website": d.Website, "linkedinUrl": d.LinkedinURL, "industry": d.Industry, "size": d.Size,
		"phone": d.Phone, "address": d.Address, "city": d.City, "country": d.Country,
		"registrationNumber": d.RegistrationNumber, "about": d.About,
	}
}
