package candidates

// Use cases: each public method is one thing a user can do. Each runs in one
// tenant-scoped transaction, applies the domain rules, writes the audit log,
// and decides which fields the user may see.

import (
	"context"
	"time"

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
	pool        *pgxpool.Pool
	phoneRegion string
	users       Users
	rules       FieldRules // the company's mandatory fields and sources
	movers      []RecordMover
	notes       Notes // screening calls' notes
}

// Create adds a candidate.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Profile) (View, error) {
	profile, errs := NormalizeProfile(in, s.phoneRegion)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
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
		view = View{Candidate: fromRow(row), CompensationRestricted: !canSeeSalary(p)}
		view.OwnerName, err = ownerName(ctx, tx, s.users, view.OwnerID)
		return err
	})
	return view, err
}

// Get returns one candidate, with salary details only if p may see them.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (View, error) {
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetCandidate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("candidate")
		}
		if err != nil {
			return err
		}
		view, err = loadView(ctx, tx, s.users, p, fromRow(row))
		return err
	})
	return view, err
}

// Summary is a candidate as shown in lists.
type Summary struct {
	ID                      string
	FullName                string
	Email                   *string
	Phone                   *string
	CurrentTitle            *string
	CurrentCompany          *string
	Location                *string
	WorkAuthorizationStatus *string
	Tags                    []string
	UpdatedAt               time.Time
}

// Page is one page of a list, newest first.
type Page struct {
	Items      []Summary
	NextCursor *string
}

// Filter narrows a list: text, tag and skill (each optional).
type Filter struct {
	Query, Tag, Skill *string
	Owner             *string // a user id
	// Similar: names like Query (typos), the closest first, one page only.
	Similar bool
}

// List returns up to limit candidates matching f, starting after cursor (the
// last ID of the previous page).
func (s *Service) List(ctx context.Context, p authz.Principal, f Filter, cursor *string, limit int) (Page, error) {
	var page Page
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		similar := f.Similar && f.Query != nil
		if similar {
			cursor = nil // ordered by likeness, not by id: one page
		}
		rows, err := store.New(tx).ListCandidates(ctx, store.ListCandidatesParams{
			Cursor: cursor, Query: f.Query, Tag: f.Tag, Skill: f.Skill, Owner: f.Owner, SimilarNames: similar, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
		})
		if err != nil {
			return err
		}
		if similar && len(rows) > limit {
			rows = rows[:limit]
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1].ID
			page.NextCursor = &last
		}
		page.Items = make([]Summary, len(rows))
		for i, r := range rows {
			page.Items[i] = Summary{
				ID: r.ID, FullName: FullName(Profile{FirstName: r.FirstName, LastName: r.LastName}),
				Email: r.Email, Phone: r.Phone, CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany,
				Location: r.Location, WorkAuthorizationStatus: r.WorkAuthorization, Tags: r.Tags, UpdatedAt: r.UpdatedAt,
			}
		}
		return nil
	})
	return page, err
}

// Update changes some fields of a candidate. expectedVersion must be the
// version the user loaded, or the update is refused (no lost updates).
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch ProfilePatch) (View, error) {
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
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
	if unchangedOK && len(errs) == 0 && len(audit.Diff(profileFields(before), profileFields(after))) == 0 {
		return loadView(ctx, tx, s.users, p, fromRow(row))
	}
	if int(row.Version) != expectedVersion {
		return View{}, apperr.StaleVersion("candidate").WithDetail("currentVersion", row.Version)
	}
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	if err := s.checkFields(ctx, tx, &before, after); err != nil {
		return View{}, err
	}
	if err := s.checkEmailFree(ctx, tx, after.Email, id); err != nil {
		return View{}, err
	}
	updated, err := q.UpdateCandidate(ctx, updateParams(id, expectedVersion, after))
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
	return loadView(ctx, tx, s.users, p, fromRow(updated))
}

// SetCompensation replaces a candidate's salary details. expectedVersion is 0
// when none exist yet, otherwise the version the user loaded.
func (s *Service) SetCompensation(ctx context.Context, p authz.Principal, candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange) (Compensation, error) {
	if err := ValidateCompensation(current, expected).Err(); err != nil {
		return Compensation{}, err
	}
	var result Compensation
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		result, err = setCompensationIn(ctx, tx, candidateID, expectedVersion, current, expected)
		return err
	})
	return result, err
}

// setCompensationIn writes checked salary details in the caller's transaction.
func setCompensationIn(ctx context.Context, tx pgx.Tx, candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange) (Compensation, error) {
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
		row, err = q.InsertCompensation(ctx, insertCompensationParams(candidateID, current, expected))
	} else {
		row, err = q.UpdateCompensation(ctx, updateCompensationParams(candidateID, expectedVersion, current, expected))
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
func loadView(ctx context.Context, tx pgx.Tx, users Users, p authz.Principal, c Candidate) (View, error) {
	q := store.New(tx)
	history, err := loadHistory(ctx, q, c.ID)
	if err != nil {
		return View{}, err
	}
	view := View{Candidate: c, History: history}
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
	return Candidate{ID: r.ID, Version: int(r.Version), Profile: p, OwnerID: r.OwnerID, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
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
	c := Compensation{Version: int(r.Version), UpdatedAt: r.UpdatedAt}
	current, err := database.DecimalString(r.CurrentAmount)
	if err != nil {
		return Compensation{}, err
	}
	if current != nil && r.CurrentCurrency != nil && r.CurrentPeriod != nil {
		c.Current = &SalaryAmount{Amount: *current, Currency: *r.CurrentCurrency, Period: *r.CurrentPeriod}
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
		c.Expected = &SalaryRange{Min: *lo, Max: hi, Currency: *r.ExpectedCurrency, Period: *r.ExpectedPeriod}
	}
	return c, nil
}

func insertCompensationParams(candidateID string, current *SalaryAmount, expected *SalaryRange) store.InsertCompensationParams {
	u := updateCompensationParams(candidateID, 0, current, expected)
	return store.InsertCompensationParams{
		CandidateID: candidateID, CurrentAmount: u.CurrentAmount, CurrentCurrency: u.CurrentCurrency,
		CurrentPeriod: u.CurrentPeriod, ExpectedMin: u.ExpectedMin, ExpectedMax: u.ExpectedMax,
		ExpectedCurrency: u.ExpectedCurrency, ExpectedPeriod: u.ExpectedPeriod,
	}
}

func updateCompensationParams(candidateID string, expectedVersion int, current *SalaryAmount, expected *SalaryRange) store.UpdateCompensationParams {
	u := store.UpdateCompensationParams{CandidateID: candidateID, ExpectedVersion: int32(expectedVersion)} //nolint:gosec // small positive integer
	if current != nil {
		u.CurrentAmount, u.CurrentCurrency, u.CurrentPeriod = &current.Amount, &current.Currency, &current.Period
	}
	if expected != nil {
		u.ExpectedMin, u.ExpectedMax, u.ExpectedCurrency, u.ExpectedPeriod = &expected.Min, expected.Max, &expected.Currency, &expected.Period
	}
	return u
}

// UsedValues are tags, skills, industries and specializations already used in
// the company, most used first.
type UsedValues struct {
	Tags, Skills, Industries, Specializations []string
}

// Used returns the values already used, for suggestions.
func (s *Service) Used(ctx context.Context, p authz.Principal) (UsedValues, error) {
	out := UsedValues{Tags: []string{}, Skills: []string{}, Industries: []string{}, Specializations: []string{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).UsedValues(ctx)
		for _, r := range rows {
			switch r.Kind {
			case "tag":
				out.Tags = append(out.Tags, r.Value)
			case "skill":
				out.Skills = append(out.Skills, r.Value)
			case "industry":
				out.Industries = append(out.Industries, r.Value)
			case "specialization":
				out.Specializations = append(out.Specializations, r.Value)
			}
		}
		return err
	})
	return out, err
}
