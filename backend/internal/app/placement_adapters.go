package app

// Adapters giving the placements module what invoicing needs: a client's
// terms of business, the contacts who may receive invoices, and email.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
)

// placementTerms gives the placements module a client's terms of business.
type placementTerms struct{ companies *companies.Module }

func (a placementTerms) Terms(ctx context.Context, tx pgx.Tx, companyID string) (placements.ClientTerms, error) {
	t, err := a.companies.Terms(ctx, tx, companyID)
	return placements.ClientTerms{FeePercent: t.FeePercent, PaymentTermsDays: t.PaymentTermsDays, GuaranteeDays: t.GuaranteeDays, Set: t.Version > 0}, err
}

// placementContacts gives the placements module the contacts who may receive invoices.
type placementContacts struct{ contacts *contacts.Module }

func (a placementContacts) Contacts(ctx context.Context, tx pgx.Tx, ids []string) (map[string]placements.Contact, error) {
	briefs, err := a.contacts.Briefs(ctx, tx, ids)
	out := make(map[string]placements.Contact, len(briefs))
	for id, b := range briefs {
		out[id] = placements.Contact{Name: b.Name, CompanyID: b.CompanyID, Email: b.Email}
	}
	return out, err
}

// placementMailer emails invoices for the placements module.
type placementMailer struct{ emails *emails.Module }

func (a placementMailer) SendInvoice(ctx context.Context, tx pgx.Tx, tenantID string, m placements.InvoiceMail) error {
	return a.emails.Notify(ctx, tx, tenantID, emails.Notice{
		To: []string{m.To}, Subject: m.Subject, Body: m.Body, ApplicationID: &m.ApplicationID, SenderName: m.From,
		Files: []emails.NoticeFile{{Name: m.FileName, ContentType: "application/pdf", Data: m.PDF}}, Restricted: true,
	})
}
