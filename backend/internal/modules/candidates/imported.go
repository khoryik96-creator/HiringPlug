package candidates

// Candidates made from imported CVs (intake module): a placeholder named
// after the file, then filled in from the CV once it has been read.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// FromCV is what a read CV says about someone (any part may be missing).
type FromCV struct {
	FirstName, LastName string
	Profile             Profile // the other fields; FirstName and LastName unused
	History             History
}

// PlaceholderName is a candidate's first name while their CV is read: the
// file's name without its extension (e.g. "Siti Aisyah CV").
func PlaceholderName(fileName string) string {
	name := strings.TrimSpace(fileName)
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	name = strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(name)), " ")
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	if name == "" {
		name = "Imported CV"
	}
	return name
}

// CreatePlaceholder adds a candidate for an imported file, named after it,
// owned by owner (nil for none), in the caller's transaction.
func (m *Module) CreatePlaceholder(ctx context.Context, tx pgx.Tx, fileName, source string, owner *string) (string, error) {
	p := Profile{FirstName: PlaceholderName(fileName), Source: &source}
	row, err := store.New(tx).InsertCandidate(ctx, insertParams(p, owner))
	if err != nil {
		return "", err
	}
	return row.ID, audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.created", EntityType: "candidate", EntityID: row.ID, Changes: map[string]any{"via": "CV import", "source": source},
	})
}

// fillEmpty takes the CV's value for each field the profile has empty.
func fillEmpty(p, cv Profile) Profile {
	take := func(k **string, v *string) {
		if *k == nil && v != nil {
			*k = v
		}
	}
	take(&p.Phone, cv.Phone)
	take(&p.Location, cv.Location)
	take(&p.Nationality, cv.Nationality)
	take(&p.ResidenceCountry, cv.ResidenceCountry)
	take(&p.LinkedinURL, cv.LinkedinURL)
	take(&p.Industry, cv.Industry)
	take(&p.CurrentTitle, cv.CurrentTitle)
	take(&p.CurrentCompany, cv.CurrentCompany)
	if p.NoticePeriod == nil {
		p.NoticePeriod = cv.NoticePeriod
	}
	if len(p.Skills) == 0 {
		p.Skills = cv.Skills
	}
	return p
}

// FillFromCV fills an imported candidate in from their CV: the name, and
// every empty field; the work history and education when they have none.
// A CV value the rules refuse (a phone in no known format) is left out. When
// the CV's email belongs to another candidate, the email is not set and
// emailOwner names that candidate (the import then merges into them).
func (m *Module) FillFromCV(ctx context.Context, tx pgx.Tx, id string, cv FromCV) (emailOwner string, err error) {
	q := store.New(tx)
	row, err := q.GetCandidate(ctx, id)
	if err != nil {
		return "", err
	}
	before := fromRow(row).Profile
	after := fillEmpty(before, cv.Profile)
	if strings.TrimSpace(cv.FirstName) != "" {
		after.FirstName, after.LastName = cv.FirstName, cv.LastName
	}
	if e := cv.Profile.Email; e != nil && after.Email == nil {
		lower := strings.ToLower(strings.TrimSpace(*e))
		existing, err := q.FindCandidateIDByEmail(ctx, &lower)
		switch {
		case err == nil && existing != id:
			emailOwner = existing
		case err == nil || database.IsNoRows(err):
			after.Email = e
		default:
			return "", err
		}
	}
	normal, errs := NormalizeProfile(after, m.service.phoneRegion)
	for len(errs) > 0 { // leave out what the rules refuse, keep the rest
		dropped := false
		for _, e := range errs {
			dropped = dropField(&after, before, e.Field) || dropped
		}
		if !dropped {
			return "", errs.Err()
		}
		normal, errs = NormalizeProfile(after, m.service.phoneRegion)
	}
	if _, err := q.UpdateCandidate(ctx, updateParams(id, int(row.Version), normal)); err != nil {
		return "", err
	}
	if h, err := loadHistory(ctx, q, id); err != nil {
		return "", err
	} else if len(h.Work) == 0 && len(h.Education) == 0 {
		if clean, errs := NormalizeHistory(cv.History); len(errs) == 0 {
			if err := saveHistory(ctx, q, id, clean); err != nil {
				return "", err
			}
		}
	}
	return emailOwner, audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.filled_from_cv", EntityType: "candidate", EntityID: id,
		Changes: audit.Diff(profileFields(before), profileFields(normal)),
	})
}

// dropField puts a refused field back as it was; false when the field is
// not one a CV fills.
func dropField(p *Profile, before Profile, field string) bool {
	switch {
	case field == "phone":
		p.Phone = before.Phone
	case field == "email":
		p.Email = before.Email
	case field == "linkedinUrl":
		p.LinkedinURL = before.LinkedinURL
	case field == "nationality":
		p.Nationality = before.Nationality
	case field == "residenceCountry":
		p.ResidenceCountry = before.ResidenceCountry
	case strings.HasPrefix(field, "noticePeriod"):
		p.NoticePeriod = before.NoticePeriod
	case slices.Contains([]string{"firstName", "lastName"}, field):
		p.FirstName, p.LastName = before.FirstName, before.LastName
	case strings.HasPrefix(field, "skills"):
		p.Skills = before.Skills
	default:
		return false
	}
	return true
}
