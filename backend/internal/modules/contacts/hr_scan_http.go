package contacts

import (
	"context"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetHrScan handles GET /v1/companies/{companyId}/hr-scan.
func (h *HTTP) GetHrScan(ctx context.Context, req api.GetHrScanRequestObject) (api.GetHrScanResponseObject, error) {
	scan, err := h.svc.HRScan(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	out := api.GetHrScan200JSONResponse{Scan: nullable.NewNullNullable[api.HrScan]()}
	if scan != nil {
		out.Scan = nullable.NewNullableWithValue(*scan)
	}
	return out, nil
}

// StartHrScan handles PUT /v1/companies/{companyId}/hr-scan.
func (h *HTTP) StartHrScan(ctx context.Context, req api.StartHrScanRequestObject) (api.StartHrScanResponseObject, error) {
	scan, err := h.svc.StartHRScan(ctx, authz.MustFromContext(ctx), req.CompanyId, *req.Body)
	if err != nil {
		return nil, err
	}
	return api.StartHrScan200JSONResponse(scan), nil
}

// AddHrScanPage handles POST /v1/companies/{companyId}/hr-scan/pages.
func (h *HTTP) AddHrScanPage(ctx context.Context, req api.AddHrScanPageRequestObject) (api.AddHrScanPageResponseObject, error) {
	scan, err := h.svc.AddHRScanPage(ctx, authz.MustFromContext(ctx), req.CompanyId, *req.Body)
	if err != nil {
		return nil, err
	}
	return api.AddHrScanPage200JSONResponse(scan), nil
}

// AddHrScanJoined handles POST /v1/companies/{companyId}/hr-scan/joined.
func (h *HTTP) AddHrScanJoined(ctx context.Context, req api.AddHrScanJoinedRequestObject) (api.AddHrScanJoinedResponseObject, error) {
	scan, err := h.svc.AddHRScanJoined(ctx, authz.MustFromContext(ctx), req.CompanyId, *req.Body)
	if err != nil {
		return nil, err
	}
	return api.AddHrScanJoined200JSONResponse(scan), nil
}
