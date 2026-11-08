package candidates

// HTTP handlers. Each one converts the generated contract types to domain
// values, calls the service, and converts the result back. The contract
// middleware has already checked sign-in, permission and input shape.

import (
	"context"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

const defaultPageSize = 25

// HTTP implements the candidates operations of the contract.
type HTTP struct{ svc *Service }

// ListCandidates handles GET /v1/candidates.
func (h *HTTP) ListCandidates(ctx context.Context, req api.ListCandidatesRequestObject) (api.ListCandidatesResponseObject, error) {
	limit := defaultPageSize
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	p := authz.MustFromContext(ctx)
	owner := req.Params.Owner
	if owner != nil && *owner == "me" {
		owner = &p.UserID
	}
	page, err := h.svc.List(ctx, p, Filter{Query: req.Params.Q, Tag: req.Params.Tag, Skill: req.Params.Skill, Owner: owner, Similar: req.Params.Similar != nil && *req.Params.Similar}, req.Params.Cursor, limit)
	if err != nil {
		return nil, err
	}
	out := api.ListCandidates200JSONResponse{Items: make([]api.CandidateSummary, len(page.Items)), NextCursor: orNull(page.NextCursor)}
	for i, s := range page.Items {
		out.Items[i] = summaryToAPI(s)
	}
	return out, nil
}

func summaryToAPI(s Summary) api.CandidateSummary {
	out := api.CandidateSummary{
		Id: s.ID, FullName: s.FullName, Email: orNull(s.Email), Phone: orNull(s.Phone),
		CurrentTitle: orNull(s.CurrentTitle), CurrentCompany: orNull(s.CurrentCompany), Location: orNull(s.Location),
		WorkAuthorizationStatus: orNull(convertPtr[api.WorkAuthorizationStatus](s.WorkAuthorizationStatus)),
		WorkAuthorizationBasis:  orNull(convertPtr[api.WorkAuthorizationBasis](s.WorkAuthorizationBasis)),
		Nationality:             orNull(s.Nationality),
		ResidenceCountry:        orNull(s.ResidenceCountry),
		Source:                  orNull(s.Source),
		Industry:                orNull(s.Industry),
		Tags:                    nonNil(s.Tags), UpdatedAt: s.UpdatedAt,
		Availability:      orNull(convertPtr[api.CandidateAvailability](s.Availability)),
		AvailabilitySince: orNull(s.AvailabilitySince),
		NoticePeriod:      nullable.NewNullNullable[api.NoticePeriod](),
		CurrentSalary:     nullable.NewNullNullable[api.SalaryAmount](),
		ExpectedSalary:    nullable.NewNullNullable[api.SalaryRange](),
		SalaryRestricted:  s.SalaryRestricted,
	}
	if np := s.NoticePeriod; np != nil {
		out.NoticePeriod = nullable.NewNullableWithValue(api.NoticePeriod{Value: np.Value, Unit: api.NoticePeriodUnit(np.Unit)})
	}
	if s.Compensation != nil {
		c := compensationToAPI(*s.Compensation)
		out.CurrentSalary, out.ExpectedSalary = c.CurrentSalary, c.ExpectedSalary
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListCandidateUsedValues handles GET /v1/candidates/used-values.
func (h *HTTP) ListCandidateUsedValues(ctx context.Context, _ api.ListCandidateUsedValuesRequestObject) (api.ListCandidateUsedValuesResponseObject, error) {
	u, err := h.svc.Used(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.ListCandidateUsedValues200JSONResponse{Tags: u.Tags, Skills: u.Skills, Industries: u.Industries, Specializations: u.Specializations}, nil
}

// CreateCandidate handles POST /v1/candidates.
func (h *HTTP) CreateCandidate(ctx context.Context, req api.CreateCandidateRequestObject) (api.CreateCandidateResponseObject, error) {
	b := req.Body
	in := Profile{
		FirstName:         b.FirstName,
		LastName:          deref(b.LastName),
		Email:             emailPtr(b.Email),
		Phone:             valueOrNil(b.Phone),
		Nationality:       valueOrNil(b.Nationality),
		ResidenceCountry:  valueOrNil(b.ResidenceCountry),
		LinkedinURL:       valueOrNil(b.LinkedinUrl),
		Availability:      convertPtr[string](valueOrNil(b.Availability)),
		WorkAuthorization: workAuthorizationFromAPI(b.WorkAuthorization),
		NoticePeriod:      noticePeriodFromAPI(b.NoticePeriod),
		Industry:          valueOrNil(b.Industry),
		Specialization:    valueOrNil(b.Specialization),
		CurrentTitle:      valueOrNil(b.CurrentTitle),
		CurrentCompany:    valueOrNil(b.CurrentCompany),
		Tags:              deref(b.Tags),
		Skills:            deref(b.Skills),
		Location:          valueOrNil(b.Location),
		Source:            convertPtr[string](valueOrNil(b.Source)),
	}
	view, err := h.svc.Create(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.CreateCandidate201JSONResponse(candidateToAPI(view)), nil
}

// GetCandidate handles GET /v1/candidates/{candidateId}.
func (h *HTTP) GetCandidate(ctx context.Context, req api.GetCandidateRequestObject) (api.GetCandidateResponseObject, error) {
	view, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	return api.GetCandidate200JSONResponse(candidateToAPI(view)), nil
}

// UpdateCandidate handles PATCH /v1/candidates/{candidateId}.
func (h *HTTP) UpdateCandidate(ctx context.Context, req api.UpdateCandidateRequestObject) (api.UpdateCandidateResponseObject, error) {
	b := req.Body
	patch := ProfilePatch{
		FirstName:         Change[string]{Set: b.FirstName != nil, Value: deref(b.FirstName)},
		LastName:          Change[string]{Set: b.LastName != nil, Value: deref(b.LastName)},
		Email:             change(b.Email, func(e openapi_types.Email) string { return string(e) }),
		Phone:             change(b.Phone, identity[string]),
		Nationality:       change(b.Nationality, identity[string]),
		ResidenceCountry:  change(b.ResidenceCountry, identity[string]),
		LinkedinURL:       change(b.LinkedinUrl, identity[string]),
		Availability:      change(b.Availability, func(a api.CandidateAvailability) string { return string(a) }),
		WorkAuthorization: Change[*WorkAuthorization]{Set: b.WorkAuthorization.IsSpecified(), Value: workAuthorizationFromAPI(b.WorkAuthorization)},
		NoticePeriod:      Change[*NoticePeriod]{Set: b.NoticePeriod.IsSpecified(), Value: noticePeriodFromAPI(b.NoticePeriod)},
		Industry:          change(b.Industry, identity[string]),
		Specialization:    change(b.Specialization, identity[string]),
		CurrentTitle:      change(b.CurrentTitle, identity[string]),
		CurrentCompany:    change(b.CurrentCompany, identity[string]),
		Location:          change(b.Location, identity[string]),
		Source:            change(b.Source, func(s api.CandidateSource) string { return s }),
		Tags:              Change[[]string]{Set: b.Tags != nil, Value: deref(b.Tags)},
		Skills:            Change[[]string]{Set: b.Skills != nil, Value: deref(b.Skills)},
	}
	view, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.CandidateId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateCandidate200JSONResponse(candidateToAPI(view)), nil
}

// PutCandidateCompensation handles PUT /v1/candidates/{candidateId}/compensation.
func (h *HTTP) PutCandidateCompensation(ctx context.Context, req api.PutCandidateCompensationRequestObject) (api.PutCandidateCompensationResponseObject, error) {
	current, expected := salaryFromAPI(*req.Body)
	comp, err := h.svc.SetCompensation(ctx, authz.MustFromContext(ctx), req.CandidateId, req.Body.ExpectedVersion, current, expected)
	if err != nil {
		return nil, err
	}
	return api.PutCandidateCompensation200JSONResponse(compensationToAPI(comp)), nil
}

func salaryFromAPI(b api.CompensationUpdate) (current *SalaryAmount, expected *SalaryRange) {
	if v, err := b.CurrentSalary.Get(); err == nil {
		current = &SalaryAmount{Amount: v.Amount, Currency: v.Currency, Period: string(v.Period)}
	}
	if v, err := b.ExpectedSalary.Get(); err == nil {
		expected = &SalaryRange{Min: v.Min, Max: valueOrNil(v.Max), Currency: v.Currency, Period: string(v.Period)}
	}
	return current, expected
}

// --- domain -> contract ---

func candidateToAPI(v View) api.Candidate {
	p := v.Profile
	out := api.Candidate{
		Id:                v.ID,
		Version:           v.Version,
		FirstName:         p.FirstName,
		LastName:          p.LastName,
		FullName:          FullName(p),
		Email:             orNull(convertPtr[openapi_types.Email](p.Email)),
		Phone:             orNull(p.Phone),
		Nationality:       orNull(p.Nationality),
		ResidenceCountry:  orNull(p.ResidenceCountry),
		LinkedinUrl:       orNull(p.LinkedinURL),
		Availability:      orNull(convertPtr[api.CandidateAvailability](p.Availability)),
		AvailabilitySince: orNull(v.AvailabilitySince),
		WorkAuthorization: nullable.NewNullNullable[api.WorkAuthorization](),
		NoticePeriod:      nullable.NewNullNullable[api.NoticePeriod](),
		Industry:          orNull(p.Industry),
		Specialization:    orNull(p.Specialization),
		CurrentTitle:      orNull(p.CurrentTitle),
		CurrentCompany:    orNull(p.CurrentCompany),
		Location:          orNull(p.Location),
		Source:            orNull(convertPtr[api.CandidateSource](p.Source)),
		Tags:              nonNil(p.Tags),
		Skills:            nonNil(p.Skills),
		RestrictedFields:  []api.RestrictedField{},
		OwnerId:           orNull(v.OwnerID),
		OwnerName:         orNull(v.OwnerName),
		CreatedAt:         v.CreatedAt,
		UpdatedAt:         v.UpdatedAt,
	}
	out.Experience, out.Education = historyToAPI(v.History)
	if wa := p.WorkAuthorization; wa != nil {
		out.WorkAuthorization = nullable.NewNullableWithValue(api.WorkAuthorization{
			Status: api.WorkAuthorizationStatus(wa.Status),
			Basis:  orNull(convertPtr[api.WorkAuthorizationBasis](wa.Basis)),
			Note:   orNull(wa.Note),
		})
	}
	if np := p.NoticePeriod; np != nil {
		out.NoticePeriod = nullable.NewNullableWithValue(api.NoticePeriod{Value: np.Value, Unit: api.NoticePeriodUnit(np.Unit)})
	}
	switch {
	case v.CompensationRestricted:
		// Left unspecified, so the field is omitted from the JSON entirely.
		out.RestrictedFields = append(out.RestrictedFields, api.RestrictedFieldCompensation)
	case v.Compensation == nil:
		out.Compensation = nullable.NewNullNullable[api.Compensation]()
	default:
		out.Compensation = nullable.NewNullableWithValue(compensationToAPI(*v.Compensation))
	}
	return out
}

func compensationToAPI(c Compensation) api.Compensation {
	out := api.Compensation{
		Version:        c.Version,
		UpdatedAt:      c.UpdatedAt,
		CurrentSalary:  nullable.NewNullNullable[api.SalaryAmount](),
		ExpectedSalary: nullable.NewNullNullable[api.SalaryRange](),
	}
	if s := c.Current; s != nil {
		out.CurrentSalary = nullable.NewNullableWithValue(api.SalaryAmount{Amount: s.Amount, Currency: s.Currency, Period: api.PayPeriod(s.Period)})
	}
	if r := c.Expected; r != nil {
		out.ExpectedSalary = nullable.NewNullableWithValue(api.SalaryRange{
			Min: r.Min, Max: orNull(r.Max), Currency: r.Currency, Period: api.PayPeriod(r.Period),
		})
	}
	return out
}

// --- contract -> domain ---

func workAuthorizationFromAPI(n nullable.Nullable[api.WorkAuthorization]) *WorkAuthorization {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &WorkAuthorization{
		Status: string(v.Status),
		Basis:  convertPtr[string](valueOrNil(v.Basis)),
		Note:   valueOrNil(v.Note),
	}
}

func noticePeriodFromAPI(n nullable.Nullable[api.NoticePeriod]) *NoticePeriod {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &NoticePeriod{Value: v.Value, Unit: string(v.Unit)}
}

// --- small helpers for nullable and enum types ---

// orNull returns a JSON null for nil, otherwise the value. Always use it for
// required-but-nullable response fields: an unspecified Nullable would
// serialize as the zero value instead of null.
func orNull[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}

// valueOrNil returns the value of a field that may be absent or null.
func valueOrNil[T any](n nullable.Nullable[T]) *T {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &v
}

// change converts a PATCH field: absent -> not set; null -> cleared; value -> set.
func change[T any, U any](n nullable.Nullable[T], conv func(T) U) Change[*U] {
	if !n.IsSpecified() {
		return Change[*U]{}
	}
	v, err := n.Get()
	if err != nil {
		return Change[*U]{Set: true, Value: nil}
	}
	u := conv(v)
	return Change[*U]{Set: true, Value: &u}
}

// convertPtr converts between string-based types (enums, Email) keeping nil.
func convertPtr[To ~string, From ~string](p *From) *To {
	if p == nil {
		return nil
	}
	v := To(*p)
	return &v
}

func emailPtr(n nullable.Nullable[openapi_types.Email]) *string {
	return convertPtr[string](valueOrNil(n))
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func identity[T any](v T) T { return v }

// SetCandidateOwner handles PUT /v1/candidates/{candidateId}/owner.
func (h *HTTP) SetCandidateOwner(ctx context.Context, req api.SetCandidateOwnerRequestObject) (api.SetCandidateOwnerResponseObject, error) {
	owner, _ := req.Body.OwnerId.Get()
	var ownerID *string
	if req.Body.OwnerId.IsSpecified() && !req.Body.OwnerId.IsNull() {
		ownerID = &owner
	}
	if err := h.svc.SetOwner(ctx, authz.MustFromContext(ctx), req.CandidateId, ownerID); err != nil {
		return nil, err
	}
	return api.SetCandidateOwner204Response{}, nil
}
