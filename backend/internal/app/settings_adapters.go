package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
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

// optionRenames carries a word renamed in a Settings list (specialization,
// IT skills, certifications) over to the candidates who have it and to
// everyone's favourites, and says how many candidates changed.
type optionRenames struct {
	candidates *candidates.Module
	identity   *identity.Module
}

func (r optionRenames) RenameOption(ctx context.Context, tx pgx.Tx, list, from, to string) (int, error) {
	field, kind := map[string]string{
		"it_skill": candidates.SkillsField, "certification": candidates.CertificationsField, "specialization": candidates.SpecializationField,
	}[list], map[string]string{"it_skill": "itSkill", "certification": "certification", "specialization": "specialization"}[list]
	if field == "" {
		return 0, nil
	}
	if _, err := r.identity.RenameFavourite(ctx, tx, kind, from, to); err != nil {
		return 0, err
	}
	return r.candidates.RenameWord(ctx, tx, field, from, to)
}

func (r optionRenames) RenamesSaved() { r.candidates.ForgetUsedValues() }
