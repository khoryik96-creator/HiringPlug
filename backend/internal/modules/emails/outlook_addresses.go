package emails

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// suggestOutlookAddresses lists addresses from the person's own synced mail
// (never anyone else's), most used first, as Outlook suggests them in To and Cc.
func (s *Service) suggestOutlookAddresses(ctx context.Context, p authz.Principal, typed string) (api.OutlookAddressList, error) {
	out := api.OutlookAddressList{Items: []api.OutlookAddress{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) || err == nil && box.Provider != ProviderMicrosoft {
			return nil
		}
		if err != nil {
			return err
		}
		rows, err := q.OutlookAddressSuggestions(ctx, store.OutlookAddressSuggestionsParams{MailboxID: box.ID, Own: box.Address, Typed: strings.TrimSpace(typed)})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out.Items = append(out.Items, api.OutlookAddress{Address: row.Address, Name: apiconv.OrNull(orNil(row.Name))})
		}
		return nil
	})
	return out, err
}

func (h *HTTP) SuggestOutlookAddresses(ctx context.Context, r api.SuggestOutlookAddressesRequestObject) (api.SuggestOutlookAddressesResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.suggestOutlookAddresses(ctx, p, deref(r.Params.Q))
	return api.SuggestOutlookAddresses200JSONResponse(out), err
}
