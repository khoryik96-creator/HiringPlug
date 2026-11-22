package contacts

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListSimilarContacts handles GET /v1/contacts/{contactId}/similar.
func (h *HTTP) ListSimilarContacts(ctx context.Context, req api.ListSimilarContactsRequestObject) (api.ListSimilarContactsResponseObject, error) {
	found, err := h.svc.Similar(ctx, authz.MustFromContext(ctx), req.ContactId)
	if err != nil {
		return nil, err
	}
	return api.ListSimilarContacts200JSONResponse{Items: lookalikesToAPI(found)}, nil
}

// ListContactDuplicates handles GET /v1/contacts/duplicates.
func (h *HTTP) ListContactDuplicates(ctx context.Context, _ api.ListContactDuplicatesRequestObject) (api.ListContactDuplicatesResponseObject, error) {
	groups, err := h.svc.Duplicates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListContactDuplicates200JSONResponse{Items: make([]api.ContactDuplicateGroup, len(groups))}
	for i, g := range groups {
		out.Items[i] = api.ContactDuplicateGroup{Reasons: g.Reasons, Contacts: lookalikesToAPI(g.Contacts)}
	}
	return out, nil
}

// MergeContact handles POST /v1/contacts/{contactId}/merge.
func (h *HTTP) MergeContact(ctx context.Context, req api.MergeContactRequestObject) (api.MergeContactResponseObject, error) {
	contact, err := h.svc.Merge(ctx, authz.MustFromContext(ctx), req.ContactId, req.Body.DuplicateId, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.MergeContact200JSONResponse(toAPI(contact)), nil
}

func lookalikesToAPI(found []Lookalike) []api.ContactLookalike {
	out := make([]api.ContactLookalike, len(found))
	for i, l := range found {
		out[i] = api.ContactLookalike{
			Id: l.ID, CompanyId: l.CompanyID, CompanyName: c.OrNull(l.CompanyName), FullName: l.FullName,
			Title: c.OrNull(l.Title), Email: c.OrNull(l.Email), Reasons: l.Reasons,
		}
	}
	return out
}
