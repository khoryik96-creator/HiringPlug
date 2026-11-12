package candidates

// Use cases: each public method is one thing a user can do. Each runs in one
// tenant-scoped transaction, applies the domain rules, writes the audit log,
// and decides which fields the user may see.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const emailUniqueIndex = "candidate_tenant_email_uq"

// Service holds the candidate use cases.
type Service struct {
	industries  IndustryCatalogue
	pool        *pgxpool.Pool
	phoneRegion string
	users       Users
	rules       FieldRules // the company's mandatory fields and sources
	salaryRules SalaryRules
	movers      []RecordMover
	notes       Notes // screening calls' notes
	cvs         CVs   // CV text, for advanced search
}

// Create adds a candidate.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Profile) (View, error) {
	profile, errs := NormalizeProfile(in, s.phoneRegion)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		q := store.New(tx)
		if s.industries != nil {
			var e error
			profile.Industry, e = s.industries.SummaryIn(ctx, tx, profile.IndustrySelections, profile.Industry)
			if e != nil {
				return e
			}
		}
		if err := s.checkFields(ctx, tx, nil, profile); err != nil {
			return err
		}
		if err := s.checkEmailFree(ctx, tx, profile.Email, ""); err != nil {
			return err
		}
		row, err := q.InsertCandidate(ctx, insertParams(profile, ownerFor(p)))
		if database.IsUniqueViolation(err, emailUniqueIndex) {
			return duplicateEmail("")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.created", EntityType: "candidate", EntityID: row.ID,
		}); err != nil {
			return err
		}
		if err := s.saveIndustries(ctx, tx, row.ID, profile); err != nil {
			return err
		}
		row, err = q.GetCandidate(ctx, row.ID)
		if err != nil {
			return err
		}
		view = View{Candidate: fromRow(row), CompensationRestricted: !canSeeSalary(p)}
		if s.industries != nil {
			choices, e := s.industries.ReadIn(ctx, tx, "candidate", row.ID)
			if e != nil {
				return e
			}
			view.Profile.IndustrySelections = &choices
		}
		view.OwnerName, err = ownerName(ctx, tx, s.users, view.OwnerID)
		return err
	})
	return view, err
}

// Get returns one candidate, with salary details only if p may see them.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (View, error) {
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if _, err := s.industries.ReadIn(ctx, tx, "candidate", ""); err != nil {
				return err
			}
		}
		q := store.New(tx)
		row, err := q.GetCandidate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("candidate")
		}
		if err != nil {
			return err
		}
		view, err = s.loadView(ctx, tx, s.users, p, fromRow(row))
		return err
	})
	return view, err
}

// Update changes some fields of a candidate. expectedVersion must be the
// version the user loaded, or the update is refused (no lost updates).
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch ProfilePatch) (View, error) {
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		var err error
		view, err = s.updateIn(ctx, tx, p, id, expectedVersion, patch.Apply, false)
		return err
	})
	return view, err
}

// updateIn changes a candidate in the caller's transaction: change turns the
// profile as stored into the new one. With unchangedOK, a change that alters
// nothing writes nothing and needs no version check.
func (s *Service) updateIn(ctx context.Context, tx pgx.Tx, p authz.Principal, id string, expectedVersion int, change func(Profile) Profile, unchangedOK bool) (View, error) {
	if s.industries != nil {
		if err := s.industries.LockIn(ctx, tx); err != nil {
			return View{}, err
		}
	}
	q := store.New(tx)
	row, err := q.GetCandidate(ctx, id)
	if database.IsNoRows(err) {
		return View{}, apperr.NotFound("candidate")
	}
	if err != nil {
		return View{}, err
	}
	before := fromRow(row).Profile
	after, errs := NormalizeProfile(change(before), s.phoneRegion)
	if unchangedOK && after.IndustrySelections == nil && len(errs) == 0 && len(audit.Diff(profileFields(before), profileFields(after))) == 0 {
		return s.loadView(ctx, tx, s.users, p, fromRow(row))
	}
	if int(row.Version) != expectedVersion {
		return View{}, apperr.StaleVersion("candidate").WithDetail("currentVersion", row.Version)
	}
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	if s.industries != nil {
		after.Industry, err = s.industries.SummaryIn(ctx, tx, after.IndustrySelections, after.Industry)
		if err != nil {
			return View{}, err
		}
	}
	if err := s.checkFields(ctx, tx, &before, after); err != nil {
		return View{}, err
	}
	if err := s.checkEmailFree(ctx, tx, after.Email, id); err != nil {
		return View{}, err
	}
	_, err = q.UpdateCandidate(ctx, updateParams(id, expectedVersion, after))
	switch {
	case database.IsNoRows(err):
		return View{}, apperr.StaleVersion("candidate") // changed between our read and our write
	case database.IsUniqueViolation(err, emailUniqueIndex):
		return View{}, duplicateEmail("")
	case err != nil:
		return View{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.updated", EntityType: "candidate", EntityID: id,
		Changes: audit.Diff(profileFields(before), profileFields(after)),
	}); err != nil {
		return View{}, err
	}
	if err := s.saveIndustries(ctx, tx, id, after); err != nil {
		return View{}, err
	}
	updated, err := q.GetCandidate(ctx, id)
	if err != nil {
		return View{}, err
	}
	return s.loadView(ctx, tx, s.users, p, fromRow(updated))
}

// SetCompensation replaces a candidate's salary details. expectedVersion is 0
// when none exist yet, otherwise the version the user loaded.
func (s *Service) SetCompensation(ctx context.Context, p authz.Principal, candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange, notes Change[*string]) (Compensation, error) {
	if err := ValidateCompensation(current, expected).Err(); err != nil {
		return Compensation{}, err
	}
	var result Compensation
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.industries != nil {
			if err := s.industries.LockIn(ctx, tx); err != nil {
				return err
			}
		}
		var err error
		result, err = setCompensationIn(ctx, tx, candidateID, expectedVersion, current, expected, notes)
		return err
	})
	return result, err
}

// setCompensationIn writes checked salary details in the caller's transaction.
func setCompensationIn(ctx context.Context, tx pgx.Tx, candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange, notes Change[*string]) (Compensation, error) {
	q := store.New(tx)
	exists, err := q.CandidateExists(ctx, candidateID)
	if err != nil {
		return Compensation{}, err
	}
	if !exists {
		return Compensation{}, apperr.NotFound("candidate")
	}
	var row store.CandidateCompensation
	if expectedVersion == 0 {
		params := insertCompensationParams(candidateID, current, expected)
		params.SalaryNotes = notes.Value
		row, err = q.InsertCompensation(ctx, params)
	} else {
		params := updateCompensationParams(candidateID, expectedVersion, current, expected)
		params.SalaryNotes = notes.Value
		params.SalaryNotesSet = notes.Set
		row, err = q.UpdateCompensation(ctx, params)
	}
	if database.IsNoRows(err) {
		return Compensation{}, apperr.StaleVersion("salary record")
	}
	if err != nil {
		return Compensation{}, err
	}
	// Salary values never go into the audit log, only the fact they changed.
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.compensation_updated", EntityType: "candidate", EntityID: candidateID,
		Changes: map[string]any{"compensation": audit.Redacted},
	}); err != nil {
		return Compensation{}, err
	}
	return fromCompensationRow(row)
}

// Exists reports whether a candidate exists in the transaction's tenant.
// Other modules (e.g. documents) call it through the module's public API.
func (s *Service) Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	return store.New(tx).CandidateExists(ctx, id)
}

// Briefs returns id -> name and current title for the candidates that exist.
func (s *Service) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	rows, err := store.New(tx).CandidateBriefs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Brief, len(rows))
	for _, r := range rows {
		out[r.ID] = Brief{Name: FullName(Profile{FirstName: r.FirstName, LastName: r.LastName}), FirstName: r.FirstName, Email: r.Email, CurrentTitle: r.CurrentTitle, Source: r.Source}
	}
	return out, nil
}

func canSeeSalary(p authz.Principal) bool {
	return p.Can(api.PermissionCandidatesCompensationRead)
}

// withCompensation adds salary details for users allowed to see them. The
// database applies the same rule (row-level security), so this check and the
// database agree; the check here exists so we can tell the user "restricted"
// rather than "none recorded".
// loadView reads what the candidate page shows besides the profile: work
// history, education and (if the user may see it) salary.
func (s *Service) loadView(ctx context.Context, tx pgx.Tx, users Users, p authz.Principal, c Candidate) (View, error) {
	q := store.New(tx)
	history, err := loadHistory(ctx, q, c.ID)
	if err != nil {
		return View{}, err
	}
	view := View{Candidate: c, History: history}
	if s.industries != nil {
		choices, err := s.industries.ReadIn(ctx, tx, "candidate", c.ID)
		if err != nil {
			return View{}, err
		}
		view.Profile.IndustrySelections = &choices
	}
	if view.OwnerName, err = ownerName(ctx, tx, users, c.OwnerID); err != nil {
		return View{}, err
	}
	if !canSeeSalary(p) {
		view.CompensationRestricted = true
		return view, nil
	}
	row, err := q.GetCompensation(ctx, c.ID)
	if database.IsNoRows(err) {
		return view, nil
	}
	if err != nil {
		return View{}, err
	}
	comp, err := fromCompensationRow(row)
	if err != nil {
		return View{}, err
	}
	view.Compensation = &comp
	return view, nil
}

// checkEmailFree refuses an email another candidate has (one candidate per
// email), naming that candidate and who owns them when the user may see
// them; one they may not see is not named.
func (s *Service) checkEmailFree(ctx context.Context, tx pgx.Tx, email *string, selfID string) error {
	if email == nil {
		return nil
	}
	q := store.New(tx)
	existing, err := q.FindCandidateIDByEmail(ctx, email)
	if database.IsNoRows(err) || (err == nil && existing == selfID) {
		return nil
	}
	if err != nil {
		return err
	}
	e := duplicateEmail(existing)
	row, err := q.GetCandidate(ctx, existing)
	if err != nil {
		return err
	}
	c := fromRow(row)
	e = e.WithDetail("existingCandidateName", FullName(c.Profile))
	if owner, err := ownerName(ctx, tx, s.users, c.OwnerID); err != nil {
		return err
	} else if owner != nil {
		e = e.WithDetail("existingOwnerName", *owner)
	}
	return e
}

func duplicateEmail(existingID string) *apperr.Error {
	e := apperr.New(api.ErrorCodeCANDIDATEDUPLICATEEMAIL, "Another candidate already has this email.")
	if existingID != "" {
		e = e.WithDetail("existingCandidateId", existingID)
	}
	return e
}

// --- conversions between store rows and domain values ---

func fromRow(r store.Candidate) Candidate {
	p := Profile{
		FirstName: r.FirstName, LastName: r.LastName,
		Email: r.Email, Phone: r.Phone, Nationality: r.Nationality, ResidenceCountry: r.ResidenceCountry, LinkedinURL: r.LinkedinUrl, Availability: r.Availability,
		Industry: r.Industry, Specialization: r.Specialization,
		CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany,
		Location: r.Location, Source: r.Source, Tags: r.Tags, Skills: r.Skills,
	}
	if r.WorkAuthorization != nil {
		p.WorkAuthorization = &WorkAuthorization{Status: *r.WorkAuthorization, Basis: r.WorkAuthorizationBasis, Note: r.WorkAuthorizationNote}
	}
	if r.NoticePeriodValue != nil && r.NoticePeriodUnit != nil {
		p.NoticePeriod = &NoticePeriod{Value: int(*r.NoticePeriodValue), Unit: *r.NoticePeriodUnit}
	}
	return Candidate{ID: r.ID, Version: int(r.Version), Profile: p, OwnerID: r.OwnerID, AvailabilitySince: r.AvailabilitySince,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// insertParams are a new candidate's columns; owner is who they belong to
// (nil for none).
func insertParams(p Profile, owner *string) store.InsertCandidateParams {
	u := updateParams("", 0, p)
	return store.InsertCandidateParams{
		FirstName: u.FirstName, LastName: u.LastName, Email: u.Email, Phone: u.Phone, Nationality: u.Nationality,
		ResidenceCountry: u.ResidenceCountry, LinkedinUrl: u.LinkedinUrl, Availability: u.Availability,
		WorkAuthorization: u.WorkAuthorization, WorkAuthorizationBasis: u.WorkAuthorizationBasis,
		WorkAuthorizationNote: u.WorkAuthorizationNote, NoticePeriodValue: u.NoticePeriodValue,
		NoticePeriodUnit: u.NoticePeriodUnit, Industry: u.Industry, Specialization: u.Specialization,
		CurrentTitle: u.CurrentTitle, CurrentCompany: u.CurrentCompany, Location: u.Location, Source: u.Source,
		Tags: u.Tags, Skills: u.Skills, OwnerID: owner,
	}
}

func updateParams(id string, expectedVersion int, p Profile) store.UpdateCandidateParams {
	u := store.UpdateCandidateParams{
		ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small positive integers
		FirstName: p.FirstName, LastName: p.LastName, Email: p.Email, Phone: p.Phone, Nationality: p.Nationality,
		ResidenceCountry: p.ResidenceCountry, LinkedinUrl: p.LinkedinURL, Availability: p.Availability,
		Industry: p.Industry, Specialization: p.Specialization, CurrentTitle: p.CurrentTitle,
		CurrentCompany: p.CurrentCompany, Location: p.Location, Source: p.Source,
		Tags: CleanLabels(p.Tags), Skills: CleanLabels(p.Skills),
	}
	if wa := p.WorkAuthorization; wa != nil {
		u.WorkAuthorization, u.WorkAuthorizationBasis, u.WorkAuthorizationNote = &wa.Status, wa.Basis, wa.Note
	}
	if np := p.NoticePeriod; np != nil {
		v := int32(np.Value) //nolint:gosec // 0..365 per the contract
		u.NoticePeriodValue, u.NoticePeriodUnit = &v, &np.Unit
	}
	return u
}

// profileFields flattens a profile for the audit diff.
func profileFields(p Profile) map[string]any {
	m := map[string]any{
		"firstName": p.FirstName, "lastName": p.LastName, "email": p.Email, "phone": p.Phone,
		"nationality": p.Nationality, "residenceCountry": p.ResidenceCountry, "linkedinUrl": p.LinkedinURL, "availability": p.Availability, "industry": p.Industry, "specialization": p.Specialization,
		"currentTitle": p.CurrentTitle, "currentCompany": p.CurrentCompany, "location": p.Location,
		"source": p.Source, "workAuthorization": nil, "noticePeriod": nil,
		"tags": CleanLabels(p.Tags), "skills": CleanLabels(p.Skills),
	}
	if wa := p.WorkAuthorization; wa != nil {
		m["workAuthorization"] = map[string]any{"status": wa.Status, "basis": wa.Basis, "note": wa.Note}
	}
	if np := p.NoticePeriod; np != nil {
		m["noticePeriod"] = map[string]any{"value": np.Value, "unit": np.Unit}
	}
	return m
}

func fromCompensationRow(r store.CandidateCompensation) (Compensation, error) {
	c := Compensation{Version: int(r.Version), UpdatedAt: r.UpdatedAt, SalaryNotes: r.SalaryNotes}
	current, err := database.DecimalString(r.CurrentAmount)
	if err != nil {
		return Compensation{}, err
	}
	if current != nil && r.CurrentCurrency != nil && r.CurrentPeriod != nil {
		c.Current = &SalaryAmount{Description: r.CurrentDescription, Amount: *current, Currency: *r.CurrentCurrency, Period: *r.CurrentPeriod}
	}
	lo, err := database.DecimalString(r.ExpectedMin)
	if err != nil {
		return Compensation{}, err
	}
	hi, err := database.DecimalString(r.ExpectedMax)
	if err != nil {
		return Compensation{}, err
	}
	if lo != nil && r.ExpectedCurrency != nil && r.ExpectedPeriod != nil {
		c.Expected = &SalaryRange{Description: r.ExpectedDescription, Min: *lo, Max: hi, Currency: *r.ExpectedCurrency, Period: *r.ExpectedPeriod}
	}
	return c, nil
}

func insertCompensationParams(candidateID string, current *SalaryAmount, expected *SalaryRange) store.InsertCompensationParams {
	u := updateCompensationParams(candidateID, 0, current, expected)
	return store.InsertCompensationParams{
		CandidateID: candidateID, CurrentDescription: u.CurrentDescription, ExpectedDescription: u.ExpectedDescription, CurrentAmount: u.CurrentAmount, CurrentCurrency: u.CurrentCurrency,
		CurrentPeriod: u.CurrentPeriod, ExpectedMin: u.ExpectedMin, ExpectedMax: u.ExpectedMax,
		ExpectedCurrency: u.ExpectedCurrency, ExpectedPeriod: u.ExpectedPeriod,
	}
}

func updateCompensationParams(candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange) store.UpdateCompensationParams {
	u := store.UpdateCompensationParams{CandidateID: candidateID, ExpectedVersion: int32(expectedVersion)} //nolint:gosec // small positive integer
	if current != nil {
		u.CurrentDescription = current.Description
		u.CurrentAmount, u.CurrentCurrency, u.CurrentPeriod = &current.Amount, &current.Currency, &current.Period
	}
	if expected != nil {
		u.ExpectedDescription = expected.Description
		u.ExpectedMin, u.ExpectedMax, u.ExpectedCurrency, u.ExpectedPeriod = &expected.Min, expected.Max, &expected.Currency, &expected.Period
	}
	return u
}
