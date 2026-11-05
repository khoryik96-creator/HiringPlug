package settings

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// GetFieldSettings handles GET /v1/settings/fields.
func (h *HTTP) GetFieldSettings(ctx context.Context, _ api.GetFieldSettingsRequestObject) (api.GetFieldSettingsResponseObject, error) {
	s, err := h.svc.Get(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetFieldSettings200JSONResponse(settingsToAPI(s)), nil
}

// PutFieldList handles PUT /v1/settings/fields/lists/{fieldList}.
func (h *HTTP) PutFieldList(ctx context.Context, req api.PutFieldListRequestObject) (api.PutFieldListResponseObject, error) {
	in := make([]OptionInput, len(req.Body.Options))
	for i, o := range req.Body.Options {
		in[i] = OptionInput{Label: o.Label}
		if o.Key != nil {
			in[i].Key = *o.Key
		}
		if o.Hidden != nil {
			in[i].Hidden = *o.Hidden
		}
	}
	l, err := h.svc.PutList(ctx, authz.MustFromContext(ctx), string(req.FieldList), in)
	if err != nil {
		return nil, err
	}
	return api.PutFieldList200JSONResponse(listToAPI(l)), nil
}

// PutMandatoryFields handles PUT /v1/settings/fields/mandatory.
func (h *HTTP) PutMandatoryFields(ctx context.Context, req api.PutMandatoryFieldsRequestObject) (api.PutMandatoryFieldsResponseObject, error) {
	in := make([]RuleInput, len(req.Body.Fields))
	for i, f := range req.Body.Fields {
		in[i] = RuleInput{Form: string(f.Form), Field: f.Field, Mandatory: f.Mandatory}
	}
	s, err := h.svc.PutRules(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.PutMandatoryFields200JSONResponse(settingsToAPI(s)), nil
}

func listToAPI(l List) api.FieldList {
	out := api.FieldList{Name: api.FieldListName(l.Name), Label: l.Label, Kind: api.FieldListKindOpen, Options: make([]api.FieldOption, len(l.Options))}
	if l.Fixed {
		out.Kind = api.FieldListKindFixed
	}
	for i, o := range l.Options {
		out.Options[i] = api.FieldOption{Key: o.Key, Label: o.Label, Hidden: o.Hidden, BuiltIn: o.BuiltIn}
	}
	return out
}

func settingsToAPI(s Settings) api.FieldSettings {
	out := api.FieldSettings{Lists: make([]api.FieldList, len(s.Lists)), Mandatory: make([]api.MandatoryField, len(s.Fields))}
	for i, l := range s.Lists {
		out.Lists[i] = listToAPI(l)
	}
	for i, f := range s.Fields {
		out.Mandatory[i] = api.MandatoryField{Form: api.FieldForm(f.Form), Field: f.Field, Label: f.Label, Mandatory: f.Mandatory, AlwaysRequired: f.AlwaysReqd}
	}
	return out
}
