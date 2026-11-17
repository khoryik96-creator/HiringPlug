package contacts

// Find HR: a company's scan of LinkedIn people in HR. The companion reads each
// page in the recruiter's browser and sends it here; the scan keeps what it
// found and where it goes on, so a page that fails is read again on resume.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) hrCompany(ctx context.Context, tx pgx.Tx, companyID string) error {
	ok, err := s.companies.Exists(ctx, tx, companyID)
	if err == nil && !ok {
		return apperr.NotFound("company")
	}
	return err
}

// HRScan returns a company's scan (nil before the first).
func (s *Service) HRScan(ctx context.Context, p authz.Principal, companyID string) (*api.HrScan, error) {
	var out *api.HrScan
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.hrCompany(ctx, tx, companyID); err != nil {
			return err
		}
		row, err := store.New(tx).GetHrScan(ctx, companyID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		scan, err := hrScanToAPI(ctx, tx, row)
		out = &scan
		return err
	})
	return out, err
}

// StartHRScan starts a new scan of a company (replacing the last).
func (s *Service) StartHRScan(ctx context.Context, p authz.Principal, companyID string, in api.HrScanStart) (api.HrScan, error) {
	var out api.HrScan
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.hrCompany(ctx, tx, companyID); err != nil {
			return err
		}
		var country *string
		if in.Country != nil {
			c := string(*in.Country)
			country = &c
		}
		keywords := make([]string, 0, len(in.Keywords))
		for _, k := range in.Keywords {
			if k = strings.TrimSpace(k); k != "" {
				keywords = append(keywords, k)
			}
		}
		if len(HRQueries(keywords)) == 0 {
			var errs apperr.FieldErrors
			errs.Add("keywords", "add at least one job title to search for")
			return errs.Err()
		}
		row, err := store.New(tx).StartHrScan(ctx, store.StartHrScanParams{
			CompanyID: companyID, LinkedinCompanyID: in.LinkedinCompanyId, LinkedinCompanyName: strings.TrimSpace(in.LinkedinCompanyName),
			Country: country, Keywords: keywords, Pages: int32(min(in.Pages, 10)), //nolint:gosec // 1-10 (the contract)
		})
		if err != nil {
			return err
		}
		out, err = hrScanToAPI(ctx, tx, row)
		return err
	})
	return out, err
}

// AddHRScanPage records one page the companion read (or why it could not):
// the scan moves on, or stops at that page to resume there.
func (s *Service) AddHRScanPage(ctx context.Context, p authz.Principal, companyID string, in api.HrScanPage) (api.HrScan, error) {
	return s.updateHRScan(ctx, p, companyID, func(row *store.HrScan, people []HRPerson) ([]HRPerson, error) {
		if row.Status == "done" || int(row.NextQuery) != in.Query || int(row.NextPage) != in.Page {
			return nil, apperr.StaleVersion("scan") // this page was read already, or the scan started again
		}
		if in.Error != nil {
			row.Status, row.Error = "stopped", in.Error
			return people, nil
		}
		var page []HRPerson
		if in.People != nil {
			for _, r := range *in.People {
				page = append(page, HRPerson{URL: r.Url, Name: strings.TrimSpace(r.Name), Title: strings.TrimSpace(r.Title), Location: strings.TrimSpace(deref(r.Location))})
			}
		}
		people = MergeHRPeople(people, page)
		q, pg, done := HRNext(in.Query, in.Page, int(row.Pages), len(HRQueries(row.Keywords)), in.HasNext != nil && *in.HasNext)
		row.NextQuery, row.NextPage, row.PagesRead, row.Error = int32(min(q, 12)), int32(min(pg, 10)), row.PagesRead+1, nil //nolint:gosec // at most 12 searches of 10 pages
		row.Status = "running"
		if done {
			row.Status = "done"
		}
		return people, nil
	})
}

// AddHRScanJoined records a person's dates at the company, read from their profile.
func (s *Service) AddHRScanJoined(ctx context.Context, p authz.Principal, companyID string, in api.HrScanJoined) (api.HrScan, error) {
	return s.updateHRScan(ctx, p, companyID, func(_ *store.HrScan, people []HRPerson) ([]HRPerson, error) {
		for i := range people {
			if people[i].URL == in.Url {
				people[i].Joined, people[i].Left = nullableString(in.Joined), nullableString(in.Left)
				if v, err := in.Current.Get(); err == nil {
					people[i].Current = &v
				} else {
					people[i].Current = nil
				}
				return people, nil
			}
		}
		return nil, apperr.NotFound("person in the scan")
	})
}

func (s *Service) updateHRScan(ctx context.Context, p authz.Principal, companyID string,
	change func(row *store.HrScan, people []HRPerson) ([]HRPerson, error)) (api.HrScan, error) {
	var out api.HrScan
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.LockHrScan(ctx, companyID)
		if database.IsNoRows(err) {
			return apperr.NotFound("scan")
		}
		if err != nil {
			return err
		}
		var people []HRPerson
		if err := json.Unmarshal(row.People, &people); err != nil {
			return err
		}
		if people, err = change(&row, people); err != nil {
			return err
		}
		raw, err := json.Marshal(people)
		if err != nil {
			return err
		}
		saved, err := q.SaveHrScan(ctx, store.SaveHrScanParams{CompanyID: companyID, Status: row.Status, NextQuery: row.NextQuery,
			NextPage: row.NextPage, PagesRead: row.PagesRead, Error: row.Error, People: raw})
		if err != nil {
			return err
		}
		out, err = hrScanToAPI(ctx, tx, saved)
		return err
	})
	return out, err
}

func nullableString(v interface{ Get() (string, error) }) *string {
	if s, err := v.Get(); err == nil {
		return &s
	}
	return nil
}

// hrScanToAPI shows a scan, best first, with who is a contact already (same
// LinkedIn profile) or may be (same name, anywhere in the agency).
func hrScanToAPI(ctx context.Context, tx pgx.Tx, row store.HrScan) (api.HrScan, error) {
	var people []HRPerson
	if err := json.Unmarshal(row.People, &people); err != nil {
		return api.HrScan{}, err
	}
	SortHRPeople(people)
	urls, names := make([]string, len(people)), make([]string, len(people))
	for i, p := range people {
		urls[i], names[i] = p.URL, strings.ToLower(strings.Join(strings.Fields(p.Name), " "))
	}
	like, err := store.New(tx).ContactsLike(ctx, store.ContactsLikeParams{Urls: urls, Names: names})
	if err != nil {
		return api.HrScan{}, err
	}
	out := api.HrScan{
		CompanyId: row.CompanyID, LinkedinCompanyId: row.LinkedinCompanyID, LinkedinCompanyName: row.LinkedinCompanyName,
		Keywords: row.Keywords, Queries: HRQueries(row.Keywords), Pages: int(row.Pages), Status: api.HrScanStatus(row.Status),
		PagesRead: int(row.PagesRead), Error: apiconv.OrNull(row.Error), StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt,
		People: make([]api.HrScanPerson, len(people)),
	}
	if row.Country != nil {
		c := HRCountries[*row.Country]
		out.Country.Set(api.HrCountry(*row.Country))
		out.CountryName.Set(c.Name)
		out.GeoId.Set(c.Geo)
	} else {
		out.Country.SetNull()
		out.CountryName.SetNull()
		out.GeoId.SetNull()
	}
	if row.Status == "done" {
		out.Next.SetNull()
	} else {
		out.Next.Set(api.HrScanNext{Query: int(row.NextQuery), Page: int(row.NextPage)})
	}
	for i, p := range people {
		person := api.HrScanPerson{Url: p.URL, Name: p.Name, Title: p.Title, Location: p.Location, Tier: api.HrScanPersonTier(HRTier(p.Title)),
			Joined: apiconv.OrNull(p.Joined), Left: apiconv.OrNull(p.Left), Current: apiconv.OrNull(p.Current)}
		person.Hidden.SetNull()
		if why := HRHidden(p.Title); why != "" {
			person.Hidden.Set(why)
		}
		person.ContactId.SetNull()
		person.ContactName.SetNull()
		person.DuplicateOf.SetNull()
		person.DuplicateName.SetNull()
		for _, c := range like {
			switch {
			case c.LinkedinUrl == p.URL:
				person.ContactId.Set(c.ID)
				person.ContactName.Set(c.Name)
			case !person.DuplicateOf.IsSpecified() || person.DuplicateOf.IsNull():
				if strings.EqualFold(strings.Join(strings.Fields(c.Name), " "), names[i]) {
					person.DuplicateOf.Set(c.ID)
					person.DuplicateName.Set(c.Name)
				}
			}
		}
		if !person.ContactId.IsNull() {
			person.DuplicateOf.SetNull()
			person.DuplicateName.SetNull()
		}
		out.People[i] = person
	}
	return out, nil
}
