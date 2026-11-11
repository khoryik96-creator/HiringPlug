package settings

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// IndustryCatalogue is the shared catalogue's read-only flat compatibility view.
type IndustryCatalogue interface {
	RootOptionsIn(context.Context, pgx.Tx) (map[string]bool, error)
}

// UseIndustries removes the old industry dropdown as an independent source of truth.
func (m *Module) UseIndustries(c IndustryCatalogue) { m.service.industries = c }
func invalidIndustryList() error {
	return apperr.Validation(apperr.FieldErrors{{Field: "industry", Message: "Manage industries and subindustries in Settings > Industries."}})
}
func (s *Service) industryList(ctx context.Context, tx pgx.Tx, out *Settings) error {
	if s.industries == nil {
		return nil
	}
	values, err := s.industries.RootOptionsIn(ctx, tx)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for i := range out.Lists {
		if out.Lists[i].Name != "industry" {
			continue
		}
		out.Lists[i].Options = []Option{}
		for _, name := range names {
			out.Lists[i].Options = append(out.Lists[i].Options, Option{Key: name, Label: name, Hidden: values[name]})
		}
	}
	return nil
}
