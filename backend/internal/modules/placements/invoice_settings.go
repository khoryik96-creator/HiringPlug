package placements

// The agency's invoice template: read (the defaults until saved), saved,
// and numbered from.

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// settingsIn is the agency's template in tx: saved, or the defaults.
func (s *Service) settingsIn(ctx context.Context, tx pgx.Tx, tenantID string) (InvoiceSettings, error) {
	row, err := store.New(tx).GetInvoiceSettings(ctx)
	if database.IsNoRows(err) {
		agency, err := s.inv.Tenants.TenantName(ctx, tx, tenantID)
		if err != nil {
			return InvoiceSettings{}, err
		}
		return DefaultInvoiceSettings(agency), nil
	}
	if err != nil {
		return InvoiceSettings{}, err
	}
	return InvoiceSettings{
		Issuer: row.Issuer, BankDetails: row.BankDetails, NumberPrefix: row.NumberPrefix, NextNumber: int(row.NextNumber),
		TaxLabel: row.TaxLabel, TaxPercent: trimZeros(row.TaxPercent), LineText: row.LineText, Footer: row.Footer,
		EmailSubject: row.EmailSubject, EmailBody: row.EmailBody, SendHour: int(row.SendHour), Version: int(row.Version),
	}, nil
}

// ensureSettings saves the defaults when nothing is saved yet (invoices are
// numbered from the saved row).
func (s *Service) ensureSettings(ctx context.Context, tx pgx.Tx, tenantID string) error {
	cur, err := s.settingsIn(ctx, tx, tenantID)
	if err != nil || cur.Version > 0 {
		return err
	}
	return store.New(tx).InsertInvoiceSettings(ctx, insertParams(cur))
}

func insertParams(v InvoiceSettings) store.InsertInvoiceSettingsParams {
	return store.InsertInvoiceSettingsParams{
		Issuer: v.Issuer, BankDetails: v.BankDetails, NumberPrefix: v.NumberPrefix, NextNumber: int32(v.NextNumber), //nolint:gosec // checked
		TaxLabel: v.TaxLabel, TaxPercent: v.TaxPercent, LineText: v.LineText, Footer: v.Footer,
		EmailSubject: v.EmailSubject, EmailBody: v.EmailBody, SendHour: int32(v.SendHour), //nolint:gosec // 0..23
	}
}

// GetSettings returns the agency's invoice template.
func (s *Service) GetSettings(ctx context.Context, p authz.Principal) (InvoiceSettings, error) {
	var out InvoiceSettings
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.settingsIn(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}

// PutSettings saves the agency's invoice template. expectedVersion is the
// version the person loaded (0: the defaults).
func (s *Service) PutSettings(ctx context.Context, p authz.Principal, expectedVersion int, in InvoiceSettings) (InvoiceSettings, error) {
	in, errs := NormalizeInvoiceSettings(in)
	if err := errs.Err(); err != nil {
		return InvoiceSettings{}, err
	}
	var out InvoiceSettings
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		cur, err := s.settingsIn(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if cur.Version != expectedVersion {
			return apperr.StaleVersion("invoice settings").WithDetail("currentVersion", cur.Version)
		}
		q := store.New(tx)
		if expectedVersion == 0 {
			if err := q.InsertInvoiceSettings(ctx, insertParams(in)); err != nil {
				return err
			}
		} else {
			params := insertParams(in)
			n, err := q.UpdateInvoiceSettings(ctx, store.UpdateInvoiceSettingsParams{
				Issuer: params.Issuer, BankDetails: params.BankDetails, NumberPrefix: params.NumberPrefix, NextNumber: params.NextNumber,
				TaxLabel: params.TaxLabel, TaxPercent: params.TaxPercent, LineText: params.LineText, Footer: params.Footer,
				EmailSubject: params.EmailSubject, EmailBody: params.EmailBody, SendHour: params.SendHour,
				ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return apperr.StaleVersion("invoice settings")
			}
		}
		if out, err = s.settingsIn(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "invoice_settings.saved", EntityType: "invoice_settings", EntityID: p.TenantID,
			Changes: map[string]any{"nextNumber": strconv.Itoa(out.NextNumber), "numberPrefix": out.NumberPrefix, "taxPercent": out.TaxPercent}})
	})
	return out, err
}
