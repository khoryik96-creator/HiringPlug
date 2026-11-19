package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings"
)

// claimOptions: a claim's receipt and claim types are Settings dropdowns.
type claimOptions struct{ settings *settings.Module }

func (o claimOptions) HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error) {
	return o.settings.HasOption(ctx, tx, list, key)
}

func (o claimOptions) Choices(ctx context.Context, tx pgx.Tx, list string) ([]claims.Option, error) {
	opts, err := o.settings.Choices(ctx, tx, list)
	out := make([]claims.Option, 0, len(opts))
	for _, opt := range opts {
		out = append(out, claims.Option{Key: opt.Key, Label: opt.Label})
	}
	return out, err
}

func (o claimOptions) Labels(ctx context.Context, tx pgx.Tx) (map[string]map[string]string, error) {
	return o.settings.Labels(ctx, tx)
}

// claimStorageApps: OneDrive and Google Drive connect through the agency's
// Microsoft and Google apps (Settings > Email connection).
type claimStorageApps struct{ emails *emails.Module }

func (a claimStorageApps) App(ctx context.Context, tx pgx.Tx, tenant, provider string) (claims.App, error) {
	app, err := a.emails.StorageApplication(ctx, tx, tenant, provider)
	return claims.App{ClientID: app.ClientID, ClientSecret: app.ClientSecret}, err
}

// claimRecords: a claim is tagged to a client (company or contact) or a
// candidate; each module's queries run under its row-level security, so
// only records the person may see are named.
type claimRecords struct {
	candidates *candidates.Module
	contacts   *contacts.Module
	companies  *companies.Module
}

func (r claimRecords) Records(ctx context.Context, tx pgx.Tx, kind string, ids []string) (map[string]claims.Record, error) {
	out := map[string]claims.Record{}
	switch kind {
	case claims.LinkCandidate:
		found, err := r.candidates.Briefs(ctx, tx, ids)
		for id, b := range found {
			out[id] = claims.Record{Name: b.Name, Detail: b.CurrentTitle}
		}
		return out, err
	case claims.LinkContact:
		found, err := r.contacts.Briefs(ctx, tx, ids)
		if err != nil {
			return nil, err
		}
		companyIDs := make([]string, 0, len(found))
		for _, b := range found {
			if b.CompanyID != "" {
				companyIDs = append(companyIDs, b.CompanyID)
			}
		}
		names, err := r.companies.Names(ctx, tx, companyIDs)
		for id, b := range found {
			rec := claims.Record{Name: b.Name}
			if name, ok := names[b.CompanyID]; ok {
				rec.Detail = &name
			}
			out[id] = rec
		}
		return out, err
	case claims.LinkCompany:
		names, err := r.companies.Names(ctx, tx, ids)
		for id, name := range names {
			out[id] = claims.Record{Name: name}
		}
		return out, err
	}
	return out, nil
}
