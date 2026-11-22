package companies

// Duplicates: companies that look like the same one (the same website or
// LinkedIn page, or the same name without its legal form), and merging one
// into another.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/dupgroups"
)

// RecordMover moves another module's records about a company (its contacts,
// jobs, notes...) when two duplicates are merged, inside the merge's
// transaction.
type RecordMover interface {
	MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error
}

// MoveRecordsWith sets the modules whose records follow a merge. Called by
// internal/app once every module exists.
func (m *Module) MoveRecordsWith(movers ...RecordMover) { m.service.movers = movers }

// FillFrom returns keep with each empty field taken from dup, and the names
// of the fields filled. The name and type stay keep's.
func FillFrom(keep, dup Details) (Details, []string) {
	var filled []string
	fill := func(name string, k **string, d *string) {
		if *k == nil && d != nil {
			*k = d
			filled = append(filled, name)
		}
	}
	fill("category", &keep.Category, dup.Category)
	fill("website", &keep.Website, dup.Website)
	fill("linkedinUrl", &keep.LinkedinURL, dup.LinkedinURL)
	fill("industry", &keep.Industry, dup.Industry)
	fill("size", &keep.Size, dup.Size)
	fill("phone", &keep.Phone, dup.Phone)
	fill("address", &keep.Address, dup.Address)
	fill("city", &keep.City, dup.City)
	fill("country", &keep.Country, dup.Country)
	fill("registrationNumber", &keep.RegistrationNumber, dup.RegistrationNumber)
	fill("about", &keep.About, dup.About)
	return keep, filled
}

// Lookalike is a possible duplicate and why.
type Lookalike struct {
	ID, Name                string
	City, Website, Industry *string
	Reasons                 []api.CompanyDuplicateReason
}

// Similar returns the possible duplicates of company id.
func (s *Service) Similar(ctx context.Context, p authz.Principal, id string) ([]Lookalike, error) {
	out := []Lookalike{}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if ok, err := q.CompanyExists(ctx, id); err != nil || !ok {
			if err == nil {
				err = apperr.NotFound("company")
			}
			return err
		}
		rows, err := q.SimilarCompanies(ctx, id)
		for _, r := range rows {
			l := Lookalike{ID: r.ID, Name: r.Name, City: r.City, Website: r.Website, Industry: r.Industry, Reasons: []api.CompanyDuplicateReason{}}
			for reason, on := range map[api.CompanyDuplicateReason]bool{
				api.CompanyDuplicateReasonSameWebsite: r.SameWebsite, api.CompanyDuplicateReasonSameLinkedin: r.SameLinkedin, api.CompanyDuplicateReasonSimilarName: r.SimilarName,
			} {
				if on {
					l.Reasons = append(l.Reasons, reason)
				}
			}
			slices.Sort(l.Reasons)
			out = append(out, l)
		}
		return err
	})
	return out, err
}

// DuplicateGroup is companies that look like one, and why.
type DuplicateGroup struct {
	Reasons   []api.CompanyDuplicateReason
	Companies []Lookalike
}

// Duplicates returns every group of possible duplicates, newest first. A
// company found by two reasons is in one group.
func (s *Service) Duplicates(ctx context.Context, p authz.Principal) ([]DuplicateGroup, error) {
	out := []DuplicateGroup{}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.CompanyDuplicateGroups(ctx)
		if err != nil {
			return err
		}
		groups := make([]dupgroups.Group, len(rows))
		for i, r := range rows {
			groups[i] = dupgroups.Group{Reason: r.Kind, IDs: r.Ids}
		}
		joined := dupgroups.Join(groups)
		var ids []string
		for _, g := range joined {
			ids = append(ids, g.IDs...)
		}
		found, err := q.CompanyLookalikes(ctx, ids)
		if err != nil {
			return err
		}
		byID := map[string]store.CompanyLookalikesRow{}
		for _, f := range found {
			byID[f.ID] = f
		}
		for _, g := range joined {
			dg := DuplicateGroup{Reasons: []api.CompanyDuplicateReason{}, Companies: []Lookalike{}}
			for _, r := range g.Reasons {
				dg.Reasons = append(dg.Reasons, api.CompanyDuplicateReason(r))
			}
			for _, id := range g.IDs {
				if f, ok := byID[id]; ok {
					dg.Companies = append(dg.Companies, Lookalike{ID: f.ID, Name: f.Name, City: f.City, Website: f.Website, Industry: f.Industry, Reasons: []api.CompanyDuplicateReason{}})
				}
			}
			if len(dg.Companies) > 1 {
				out = append(out, dg)
			}
		}
		return nil
	})
	return out, err
}

// Merge moves company dupID into keepID (its contacts, jobs, notes,
// documents and the rest; its terms and benefits if keepID has none; its
// values for keepID's empty fields) and deletes it, in one transaction.
// version is keepID's version as loaded.
func (s *Service) Merge(ctx context.Context, p authz.Principal, keepID, dupID string, version int) (Company, error) {
	if keepID == dupID {
		var errs apperr.FieldErrors
		errs.Add("duplicateId", "is the same company")
		return Company{}, errs.Err()
	}
	var out Company
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil { // first, as every save does (no lock inversion)
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		q := store.New(tx)
		keepRow, err := q.GetCompany(ctx, keepID)
		if database.IsNoRows(err) {
			return apperr.NotFound("company")
		}
		if err != nil {
			return err
		}
		if int(keepRow.Version) != version {
			return apperr.StaleVersion("company").WithDetail("currentVersion", keepRow.Version)
		}
		dupRow, err := q.GetCompany(ctx, dupID)
		if database.IsNoRows(err) {
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "no such company")
			return errs.Err()
		}
		if err != nil {
			return err
		}
		for _, m := range s.movers {
			if err := m.MoveCompany(ctx, tx, dupID, keepID); err != nil {
				return err
			}
		}
		for _, move := range []func(context.Context, store.MoveTermsIfNoneParams) error{
			q.MoveTermsIfNone,
			func(ctx context.Context, a store.MoveTermsIfNoneParams) error {
				return q.MoveBenefitsIfNone(ctx, store.MoveBenefitsIfNoneParams(a))
			},
			func(ctx context.Context, a store.MoveTermsIfNoneParams) error {
				return q.MoveCompanyDocuments(ctx, store.MoveCompanyDocumentsParams(a))
			},
		} {
			if err := move(ctx, store.MoveTermsIfNoneParams{FromID: dupID, ToID: keepID}); err != nil {
				return err
			}
		}
		// A mover (industries) may have changed the kept company: fill from its row now.
		if keepRow, err = q.GetCompany(ctx, keepID); err != nil {
			return err
		}
		after, filled := FillFrom(fromRow(keepRow).Details, fromRow(dupRow).Details)
		if _, err := q.DeleteMergedCompany(ctx, dupID); database.IsForeignKeyViolation(err) {
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "has jobs or contacts you cannot see: ask someone who sees them all to merge")
			return errs.Err()
		} else if err != nil {
			return err
		}
		row, err := q.UpdateCompany(ctx, store.UpdateCompanyParams{
			ID: keepID, ExpectedVersion: keepRow.Version,
			Name: after.Name, Type: after.Type, Category: after.Category, Website: after.Website, LinkedinUrl: after.LinkedinURL, Industry: after.Industry, Size: after.Size,
			Phone: after.Phone, Address: after.Address, City: after.City, Country: after.Country,
			RegistrationNumber: after.RegistrationNumber, About: after.About,
		})
		if err != nil {
			return err
		}
		for _, e := range []audit.Entry{
			{Action: "company.merged", EntityType: "company", EntityID: keepID,
				Changes: map[string]any{"duplicateId": dupID, "duplicateName": dupRow.Name, "filled": filled}},
			{Action: "company.merged_into", EntityType: "company", EntityID: dupID, Changes: map[string]any{"keptId": keepID}},
		} {
			if err := audit.Record(ctx, tx, e); err != nil {
				return err
			}
		}
		out = fromRow(row)
		return s.readIndustries(ctx, tx, &out)
	})
	return out, err
}
