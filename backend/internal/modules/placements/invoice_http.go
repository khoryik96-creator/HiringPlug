package placements

import (
	"bytes"
	"context"
	"mime"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetPlacementDefaults handles GET /v1/applications/{applicationId}/placement-defaults.
func (h *HTTP) GetPlacementDefaults(ctx context.Context, req api.GetPlacementDefaultsRequestObject) (api.GetPlacementDefaultsResponseObject, error) {
	d, err := h.svc.Defaults(ctx, authz.MustFromContext(ctx), req.ApplicationId)
	if err != nil {
		return nil, err
	}
	out := api.GetPlacementDefaults200JSONResponse{
		CompanyId: d.CompanyID, CompanyName: d.CompanyName, TermsSet: d.Terms.Set,
		FeePercent: nullable.NewNullNullable[api.DecimalAmount](), PaymentTermsDays: d.PaymentTermsDays, GuaranteeDays: d.GuaranteeDays,
		ContactId: c.OrNull(d.ContactID), SalaryCurrency: d.Terms.SalaryCurrency, Tiers: make([]api.CompanyTermsTier, len(d.Terms.Tiers)),
	}
	if out.SalaryCurrency == "" {
		out.SalaryCurrency = "MYR"
	}
	for i, t := range d.Terms.Tiers {
		out.Tiers[i] = api.CompanyTermsTier{
			SalaryFrom: c.OrNull(t.SalaryFrom), SalaryBelow: c.OrNull(t.SalaryBelow), FeePercent: c.OrNull(t.FeePercent),
			PaymentTermsDays: c.OrNull(t.PaymentTermsDays), GuaranteeDays: c.OrNull(t.GuaranteeDays), Remarks: c.OrNull(t.Remarks),
		}
	}
	if d.FeePercent != nil {
		out.FeePercent = nullable.NewNullableWithValue(*d.FeePercent)
	}
	return out, nil
}

// PutPlacementInvoicing handles PUT /v1/placements/{placementId}/invoicing.
func (h *HTTP) PutPlacementInvoicing(ctx context.Context, req api.PutPlacementInvoicingRequestObject) (api.PutPlacementInvoicingResponseObject, error) {
	b := req.Body
	in := InvoicingInput{ContactID: c.ValueOrNil(b.ContactId), PaymentTermsDays: b.PaymentTermsDays, AutoSend: b.AutoSend}
	if d, err := b.DueOn.Get(); err == nil {
		t := d.Time
		in.DueOn = &t
	}
	v, err := h.svc.PutInvoicing(ctx, authz.MustFromContext(ctx), req.PlacementId, in)
	if err != nil {
		return nil, err
	}
	return api.PutPlacementInvoicing200JSONResponse(toAPI(v)), nil
}

// SendPlacementInvoice handles POST /v1/placements/{placementId}/invoice.
func (h *HTTP) SendPlacementInvoice(ctx context.Context, req api.SendPlacementInvoiceRequestObject) (api.SendPlacementInvoiceResponseObject, error) {
	v, err := h.svc.SendNow(ctx, authz.MustFromContext(ctx), req.PlacementId)
	if err != nil {
		return nil, err
	}
	return api.SendPlacementInvoice200JSONResponse(toAPI(v)), nil
}

// GetPlacementInvoiceFile handles GET /v1/placements/{placementId}/invoice/file.
func (h *HTTP) GetPlacementInvoiceFile(ctx context.Context, req api.GetPlacementInvoiceFileRequestObject) (api.GetPlacementInvoiceFileResponseObject, error) {
	f, err := h.svc.File(ctx, authz.MustFromContext(ctx), req.PlacementId, string(req.Params.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})
	return api.GetPlacementInvoiceFile200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(f.Data), ContentLength: int64(len(f.Data)),
		Headers: api.GetPlacementInvoiceFile200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

// GetInvoiceSettings handles GET /v1/invoice-settings.
func (h *HTTP) GetInvoiceSettings(ctx context.Context, _ api.GetInvoiceSettingsRequestObject) (api.GetInvoiceSettingsResponseObject, error) {
	s, err := h.svc.GetSettings(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetInvoiceSettings200JSONResponse(settingsToAPI(s)), nil
}

// PutInvoiceSettings handles PUT /v1/invoice-settings.
func (h *HTTP) PutInvoiceSettings(ctx context.Context, req api.PutInvoiceSettingsRequestObject) (api.PutInvoiceSettingsResponseObject, error) {
	b := req.Body
	in := InvoiceSettings{
		Issuer: b.Issuer, BankDetails: b.BankDetails, NumberPrefix: b.NumberPrefix, NextNumber: b.NextNumber, TaxLabel: b.TaxLabel,
		TaxPercent: b.TaxPercent, LineText: b.LineText, Footer: b.Footer, EmailSubject: b.EmailSubject, EmailBody: b.EmailBody,
		SendHour: b.SendHour,
	}
	s, err := h.svc.PutSettings(ctx, authz.MustFromContext(ctx), b.Version, in)
	if err != nil {
		return nil, err
	}
	return api.PutInvoiceSettings200JSONResponse(settingsToAPI(s)), nil
}

func settingsToAPI(s InvoiceSettings) api.InvoiceSettings {
	fields := make([]api.InvoiceField, len(InvoiceFields))
	for i, f := range InvoiceFields {
		fields[i] = api.InvoiceField{Key: f.Key, Label: f.Label, Example: f.Example}
	}
	return api.InvoiceSettings{
		Issuer: s.Issuer, BankDetails: s.BankDetails, NumberPrefix: s.NumberPrefix, NextNumber: s.NextNumber, TaxLabel: s.TaxLabel,
		TaxPercent: s.TaxPercent, LineText: s.LineText, Footer: s.Footer, EmailSubject: s.EmailSubject, EmailBody: s.EmailBody,
		SendHour: s.SendHour, Version: s.Version, Fields: fields,
	}
}

func invoicingToAPI(v InvoicingView) api.PlacementInvoicing {
	return api.PlacementInvoicing{
		ContactId: c.OrNull(v.ContactID), ContactName: c.OrNull(v.ContactName), ContactEmail: c.OrNull(v.ContactEmail),
		PaymentTermsDays: v.PaymentTermsDays, DueOn: openapi_types.Date{Time: v.DueOn}, DueOnSet: v.DueOnSet, AutoSend: v.AutoSend,
		SendsAt: c.OrNull(v.SendsAt), SentAt: c.OrNull(v.SentAt), SendError: c.OrNull(v.SendError),
	}
}
