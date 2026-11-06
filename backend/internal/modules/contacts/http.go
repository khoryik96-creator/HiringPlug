package contacts

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

const defaultPageSize = 25

// HTTP implements the contacts operations of the contract.
type HTTP struct{ svc *Service }

// ListContacts handles GET /v1/contacts.
func (h *HTTP) ListContacts(ctx context.Context, req api.ListContactsRequestObject) (api.ListContactsResponseObject, error) {
	limit := defaultPageSize
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	page, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.Q, req.Params.CompanyId, req.Params.Cursor, req.Params.Similar != nil && *req.Params.Similar, limit)
	if err != nil {
		return nil, err
	}
	out := api.ListContacts200JSONResponse{Items: make([]api.ContactSummary, len(page.Items)), NextCursor: c.OrNull(page.NextCursor)}
	for i, s := range page.Items {
		out.Items[i] = api.ContactSummary{
			Id: s.ID, FullName: FullName(s.FirstName, s.LastName), Title: c.OrNull(s.Title),
			CompanyId: s.CompanyID, CompanyName: s.CompanyName, Email: c.OrNull(s.Email),
			EmailCheckStatus: c.OrNull(c.Convert[api.EmailCheckStatus](s.EmailCheckStatus)),
			Phone:            c.OrNull(s.Phone), Mobile: c.OrNull(s.Mobile), Status: api.ContactStatus(s.Status), UpdatedAt: s.UpdatedAt,
		}
	}
	return out, nil
}

// CreateContact handles POST /v1/contacts.
func (h *HTTP) CreateContact(ctx context.Context, req api.CreateContactRequestObject) (api.CreateContactResponseObject, error) {
	b := req.Body
	in := Details{
		CompanyID: b.CompanyId, FirstName: b.FirstName, Title: c.ValueOrNil(b.Title), Email: c.ValueOrNil(b.Email),
		Phone: c.ValueOrNil(b.Phone), Mobile: c.ValueOrNil(b.Mobile), LinkedinURL: c.ValueOrNil(b.LinkedinUrl),
		Notes: c.ValueOrNil(b.Notes),
	}
	if b.LastName != nil {
		in.LastName = *b.LastName
	}
	if b.Status != nil {
		in.Status = string(*b.Status)
	}
	contact, err := h.svc.Create(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.CreateContact201JSONResponse(toAPI(contact)), nil
}

// GetContact handles GET /v1/contacts/{contactId}.
func (h *HTTP) GetContact(ctx context.Context, req api.GetContactRequestObject) (api.GetContactResponseObject, error) {
	contact, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.ContactId)
	if err != nil {
		return nil, err
	}
	return api.GetContact200JSONResponse(toAPI(contact)), nil
}

// UpdateContact handles PATCH /v1/contacts/{contactId}.
func (h *HTTP) UpdateContact(ctx context.Context, req api.UpdateContactRequestObject) (api.UpdateContactResponseObject, error) {
	b := req.Body
	patch := Patch{
		CompanyID:   c.Required(b.CompanyId),
		FirstName:   c.Required(b.FirstName),
		LastName:    c.Required(b.LastName),
		Title:       c.Optional(b.Title, c.Same[string]),
		Email:       c.Optional(b.Email, c.Same[string]),
		Phone:       c.Optional(b.Phone, c.Same[string]),
		Mobile:      c.Optional(b.Mobile, c.Same[string]),
		LinkedinURL: c.Optional(b.LinkedinUrl, c.Same[string]),
		Status:      c.Required(c.Convert[string](b.Status)),
		Notes:       c.Optional(b.Notes, c.Same[string]),
	}
	contact, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.ContactId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateContact200JSONResponse(toAPI(contact)), nil
}

// CheckContactEmail handles POST /v1/contacts/{contactId}/email-check.
func (h *HTTP) CheckContactEmail(ctx context.Context, req api.CheckContactEmailRequestObject) (api.CheckContactEmailResponseObject, error) {
	contact, err := h.svc.CheckEmailAgain(ctx, authz.MustFromContext(ctx), req.ContactId)
	if err != nil {
		return nil, err
	}
	return api.CheckContactEmail200JSONResponse(toAPI(contact)), nil
}

func toAPI(ct Contact) api.Contact {
	d := ct.Details
	out := api.Contact{
		Id: ct.ID, Version: ct.Version, CompanyId: d.CompanyID, CompanyName: ct.CompanyName,
		FirstName: d.FirstName, LastName: d.LastName, FullName: FullName(d.FirstName, d.LastName),
		Title: c.OrNull(d.Title), Email: c.OrNull(d.Email), Phone: c.OrNull(d.Phone), Mobile: c.OrNull(d.Mobile),
		LinkedinUrl: c.OrNull(d.LinkedinURL), Status: api.ContactStatus(d.Status), Notes: c.OrNull(d.Notes),
		CreatedAt: ct.CreatedAt, UpdatedAt: ct.UpdatedAt,
	}
	var check *api.EmailCheck
	if r := ct.EmailCheck; r != nil {
		check = &api.EmailCheck{Status: api.EmailCheckStatus(r.Status), Reasons: r.Reasons, CheckedAt: r.CheckedAt}
	}
	out.EmailCheck = c.OrNull(check)
	return out
}
