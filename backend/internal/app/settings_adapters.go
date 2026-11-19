package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
)

// optionUses counts the records holding a Settings option: contacts by role,
// companies by category, claims by receipt or claim type.
type optionUses struct {
	contacts  *contacts.Module
	companies *companies.Module
	claims    *claims.Module
}

func (u optionUses) OptionUses(ctx context.Context, tx pgx.Tx, list, key string) (int, error) {
	switch list {
	case "contact_role":
		return u.contacts.CountWithRole(ctx, tx, key)
	case "company_category":
		return u.companies.CountInCategory(ctx, tx, key)
	case claims.ReceiptTypeList, claims.ClaimTypeList:
		return u.claims.OptionUses(ctx, tx, list, key)
	}
	return 0, nil
}
