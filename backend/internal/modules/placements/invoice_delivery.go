package placements

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// MayDeliverInvoice checks the queued address against the current client and
// contact before the worker hands financial information to the mail provider.
func (m *Module) MayDeliverInvoice(ctx context.Context, tx pgx.Tx, emailID string) (bool, error) {
	q := store.New(tx)
	inv, err := q.InvoiceForDelivery(ctx, &emailID)
	if database.IsNoRows(err) {
		return false, nil
	}
	if err != nil || inv.ContactID == nil || inv.DeliveryTo == nil {
		return false, err
	}
	row, err := q.GetPlacement(ctx, inv.PlacementID)
	if err != nil {
		return false, err
	}
	apps, err := m.service.applications.Briefs(ctx, tx, []string{row.ApplicationID})
	if err != nil {
		return false, err
	}
	if inv.DeliveryCompany == nil || *inv.DeliveryCompany != apps[row.ApplicationID].CompanyID {
		return false, nil
	}
	return m.service.inv.Contacts.LockInvoiceRecipient(ctx, tx, *inv.ContactID, *inv.DeliveryCompany, *inv.DeliveryTo)
}

func (m *Module) InvoiceDelivered(ctx context.Context, tx pgx.Tx, emailID string, failed bool) error {
	q := store.New(tx)
	inv, err := q.InvoiceForDelivery(ctx, &emailID)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var reason *string
	action := "placement.invoice_sent"
	if failed {
		msg := "Check the billing contact and email address, then retry."
		reason, action = &msg, "placement.invoice_failed"
	}
	if err := q.FinishInvoiceDelivery(ctx, store.FinishInvoiceDeliveryParams{DeliveryEmail: &emailID, SendError: reason}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "placement", EntityID: inv.PlacementID})
}
