package companies

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetCompanyAutofill handles GET /v1/companies/{companyId}/autofill.
func (h *HTTP) GetCompanyAutofill(ctx context.Context, req api.GetCompanyAutofillRequestObject) (api.GetCompanyAutofillResponseObject, error) {
	a, err := h.svc.Autofill(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.GetCompanyAutofill200JSONResponse(autofillToAPI(a)), nil
}

// StartCompanyAutofill handles POST /v1/companies/{companyId}/autofill.
func (h *HTTP) StartCompanyAutofill(ctx context.Context, req api.StartCompanyAutofillRequestObject) (api.StartCompanyAutofillResponseObject, error) {
	a, err := h.svc.StartAutofill(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.StartCompanyAutofill202JSONResponse(autofillToAPI(a)), nil
}

// DecideCompanySuggestion handles POST /v1/companies/{companyId}/autofill/{suggestionId}.
func (h *HTTP) DecideCompanySuggestion(ctx context.Context, req api.DecideCompanySuggestionRequestObject) (api.DecideCompanySuggestionResponseObject, error) {
	a, err := h.svc.Decide(ctx, authz.MustFromContext(ctx), req.CompanyId, req.SuggestionId, req.Body.Accept)
	if err != nil {
		return nil, err
	}
	return api.DecideCompanySuggestion200JSONResponse(autofillToAPI(a)), nil
}

func autofillToAPI(a Autofill) api.CompanyAutofill {
	out := api.CompanyAutofill{
		Status: api.CompanyAutofillStatus(a.Status), CheckedAt: apiconv.OrNull(a.CheckedAt), Error: apiconv.OrNull(a.Error),
		Suggestions: make([]api.CompanySuggestion, len(a.Suggestions)),
	}
	for i, s := range a.Suggestions {
		out.Suggestions[i] = api.CompanySuggestion{
			Id: s.ID, Field: api.CompanySuggestionField(s.Field), Value: s.Value, Current: apiconv.OrNull(s.Current), Source: s.Source,
		}
	}
	return out
}
