package market

import (
	"bytes"
	"context"
	"mime"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP holds the module's handlers.
type HTTP struct{ svc *Service }

// GetMarketMap handles GET /v1/jobs/{jobId}/market-map.
func (h *HTTP) GetMarketMap(ctx context.Context, req api.GetMarketMapRequestObject) (api.GetMarketMapResponseObject, error) {
	m, err := h.svc.Map(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	out := api.GetMarketMap200JSONResponse{
		JobId: m.JobID, JobTitle: m.JobTitle, SearchedFor: m.SearchedFor,
		Pool: api.MarketPool{
			Total: m.Pool.Total, Availability: countsToAPI(m.Pool.Availability), WorkAuthorization: countsToAPI(m.Pool.WorkAuthorization),
			Companies: countsToAPI(m.Pool.Companies), Locations: countsToAPI(m.Pool.Locations),
			MedianNoticeDays: c.OrNull(m.Pool.MedianNoticeDays), Salary: make([]api.MarketPay, len(m.Pool.Salary)),
			SalaryRestricted: m.SalaryRestricted,
		},
	}
	for i, g := range m.Pool.Salary {
		out.Pool.Salary[i] = api.MarketPay{Currency: g.Currency, Period: api.PayPeriod(g.Period), Current: rangeToAPI(g.Current), Expected: rangeToAPI(g.Expected)}
	}
	out.Insights.SetNull()
	if a := m.Insights; a != nil {
		out.Insights.Set(api.MarketInsights{AnswerId: a.ID, Status: api.AssistantAnswerStatus(a.Status), Text: c.OrNull(a.Text), Model: c.OrNull(a.Model), AskedAt: a.AskedAt})
	}
	return out, nil
}

// ExportMarketMap handles GET /v1/jobs/{jobId}/market-map/export.
func (h *HTTP) ExportMarketMap(ctx context.Context, req api.ExportMarketMapRequestObject) (api.ExportMarketMapResponseObject, error) {
	e, err := h.svc.Export(ctx, authz.MustFromContext(ctx), req.JobId, string(req.Params.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": e.FileName})
	headers := api.ExportMarketMap200ResponseHeaders{ContentDisposition: &disposition}
	if req.Params.Format == api.ExportMarketMapParamsFormatPdf {
		return api.ExportMarketMap200ApplicationpdfResponse{Body: bytes.NewReader(e.Data), ContentLength: int64(len(e.Data)), Headers: headers}, nil
	}
	return api.ExportMarketMap200ApplicationvndOpenxmlformatsOfficedocumentWordprocessingmlDocumentResponse{
		Body: bytes.NewReader(e.Data), ContentLength: int64(len(e.Data)), Headers: headers,
	}, nil
}

func countsToAPI(cs []Count) []api.MarketCount {
	out := make([]api.MarketCount, len(cs))
	for i, x := range cs {
		out[i] = api.MarketCount{Key: x.Key, Count: x.Count}
	}
	return out
}

func rangeToAPI(r *Range) nullable.Nullable[api.MarketRange] {
	if r == nil {
		return nullable.NewNullNullable[api.MarketRange]()
	}
	return nullable.NewNullableWithValue(api.MarketRange{Count: r.Count, Low: Amount(r.Low), Median: Amount(r.Median), High: Amount(r.High)})
}
