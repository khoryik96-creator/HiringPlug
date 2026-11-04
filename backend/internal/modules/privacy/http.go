package privacy

import (
	"context"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// GetPrivacyOverview handles GET /v1/privacy.
func (h *HTTP) GetPrivacyOverview(ctx context.Context, _ api.GetPrivacyOverviewRequestObject) (api.GetPrivacyOverviewResponseObject, error) {
	o, err := h.svc.Overview(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.GetPrivacyOverview200JSONResponse{
		Consent:      api.ConsentCounts{Given: o.Given, Withdrawn: o.Withdrawn, NotRecorded: o.NotRecorded},
		OpenRequests: requestsToAPI(o.Open), ClosedRequests: requestsToAPI(o.Closed), Withdrawn: []api.WithdrawnCandidate{},
	}
	for _, w := range o.WithdrawnList {
		out.Withdrawn = append(out.Withdrawn, api.WithdrawnCandidate{CandidateId: w.CandidateID, CandidateName: w.Name, Day: openapi_types.Date{Time: w.Day}})
	}
	return out, nil
}

// GetCandidatePrivacy handles GET /v1/candidates/{candidateId}/privacy.
func (h *HTTP) GetCandidatePrivacy(ctx context.Context, req api.GetCandidatePrivacyRequestObject) (api.GetCandidatePrivacyResponseObject, error) {
	cp, err := h.svc.Candidate(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.GetCandidatePrivacy200JSONResponse{Consent: nullable.NewNullNullable[api.Consent](), Requests: requestsToAPI(cp.Requests)}
	if cp.Consent != nil {
		out.Consent = nullable.NewNullableWithValue(consentToAPI(*cp.Consent))
		out.DoNotContact = cp.Consent.Status == Withdrawn
	}
	return out, nil
}

// SetCandidateConsent handles PUT /v1/candidates/{candidateId}/consent.
func (h *HTTP) SetCandidateConsent(ctx context.Context, req api.SetCandidateConsentRequestObject) (api.SetCandidateConsentResponseObject, error) {
	b := req.Body
	c, err := h.svc.SetConsent(ctx, authz.MustFromContext(ctx), req.CandidateId, b.ExpectedVersion, ConsentInput{
		Status: string(b.Status), Method: string(b.Method), Day: b.Day.Time, Note: apiconv.ValueOrNil(b.Note),
	})
	if err != nil {
		return nil, err
	}
	return api.SetCandidateConsent200JSONResponse(consentToAPI(c)), nil
}

// CreatePrivacyRequest handles POST /v1/candidates/{candidateId}/privacy-requests.
func (h *HTTP) CreatePrivacyRequest(ctx context.Context, req api.CreatePrivacyRequestRequestObject) (api.CreatePrivacyRequestResponseObject, error) {
	b := req.Body
	r, err := h.svc.CreateRequest(ctx, authz.MustFromContext(ctx), req.CandidateId, RequestInput{
		Kind: string(b.Kind), ReceivedOn: b.ReceivedOn.Time, Details: apiconv.ValueOrNil(b.Details),
	})
	if err != nil {
		return nil, err
	}
	return api.CreatePrivacyRequest201JSONResponse(requestToAPI(r)), nil
}

// DecidePrivacyRequest handles PATCH /v1/privacy-requests/{requestId}.
func (h *HTTP) DecidePrivacyRequest(ctx context.Context, req api.DecidePrivacyRequestRequestObject) (api.DecidePrivacyRequestResponseObject, error) {
	b := req.Body
	r, err := h.svc.Decide(ctx, authz.MustFromContext(ctx), req.RequestId, b.Version, Decision{Status: string(b.Status), Outcome: apiconv.ValueOrNil(b.Outcome)})
	if err != nil {
		return nil, err
	}
	return api.DecidePrivacyRequest200JSONResponse(requestToAPI(r)), nil
}

// ExportCandidateData handles GET /v1/candidates/{candidateId}/data-export.
func (h *HTTP) ExportCandidateData(ctx context.Context, req api.ExportCandidateDataRequestObject) (api.ExportCandidateDataResponseObject, error) {
	e, err := h.svc.Export(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ExportCandidateData200JSONResponse{
		CandidateId: e.CandidateID, CandidateName: e.CandidateName, Company: e.Company, ExportedAt: e.ExportedAt,
		NotIncluded: e.NotIncluded, Sections: make([]api.DataExportSection, len(e.Sections)),
	}
	for i, s := range e.Sections {
		sec := api.DataExportSection{Title: s.Title, Items: make([]api.DataExportItem, len(s.Items))}
		for j, item := range s.Items {
			fields := make([]api.DataExportField, len(item))
			for k, f := range item {
				fields[k] = api.DataExportField{Label: f.Label, Value: f.Value}
			}
			sec.Items[j] = api.DataExportItem{Fields: fields}
		}
		out.Sections[i] = sec
	}
	return out, nil
}

func consentToAPI(c Consent) api.Consent {
	return api.Consent{
		Version: c.Version, Status: api.ConsentStatus(c.Status), Method: api.ConsentMethod(c.Method), Day: openapi_types.Date{Time: c.Day},
		Note: apiconv.OrNull(c.Note), UpdatedAt: c.UpdatedAt,
	}
}

func requestsToAPI(in []RequestView) []api.PrivacyRequest {
	out := make([]api.PrivacyRequest, len(in))
	for i, r := range in {
		out[i] = requestToAPI(r)
	}
	return out
}

func requestToAPI(r RequestView) api.PrivacyRequest {
	out := api.PrivacyRequest{
		Id: r.ID, Version: r.Version, CandidateId: r.CandidateID, CandidateName: r.CandidateName, Kind: api.PrivacyRequestKind(r.Kind),
		ReceivedOn: openapi_types.Date{Time: r.ReceivedOn}, Details: apiconv.OrNull(r.Details), Status: api.PrivacyRequestStatus(r.Status),
		Outcome: apiconv.OrNull(r.Outcome), DueOn: nullable.NewNullNullable[openapi_types.Date](), Overdue: r.Overdue, DaysOpen: r.DaysOpen,
		ClosedAt: apiconv.OrNull(r.ClosedAt), ClosedByName: apiconv.OrNull(r.ClosedByName), CreatedAt: r.CreatedAt,
	}
	if r.DueOn != nil {
		out.DueOn = nullable.NewNullableWithValue(openapi_types.Date{Time: *r.DueOn})
	}
	return out
}
