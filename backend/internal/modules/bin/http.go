package bin

import (
	"context"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the bin part of the API.
type HTTP struct{ svc *Service }

func entryToAPI(e store.BinEntry) api.BinEntry {
	went := decodeWentWith(e.WentWith)
	return api.BinEntry{
		Id: e.ID, Kind: api.BinKind(e.Kind), RecordId: e.RecordID, Label: e.Label, Contacts: len(went.Contacts), Jobs: len(went.Jobs),
		BinnedByName: e.BinnedName, BinnedAt: e.BinnedAt, PurgeAfter: e.PurgeAfter,
	}
}

// ListBin handles GET /v1/bin.
func (h *HTTP) ListBin(ctx context.Context, _ api.ListBinRequestObject) (api.ListBinResponseObject, error) {
	rows, err := h.svc.List(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListBin200JSONResponse{Items: make([]api.BinEntry, len(rows))}
	for i, r := range rows {
		out.Items[i] = entryToAPI(r)
	}
	return out, nil
}

// CheckBinRecord handles GET /v1/bin/check.
func (h *HTTP) CheckBinRecord(ctx context.Context, req api.CheckBinRecordRequestObject) (api.CheckBinRecordResponseObject, error) {
	c, err := h.svc.Check(ctx, authz.MustFromContext(ctx), string(req.Params.Kind), req.Params.Id)
	if err != nil {
		return nil, err
	}
	out := api.CheckBinRecord200JSONResponse{Label: c.Label, Contacts: c.Contacts, Jobs: c.Jobs, Applications: c.Application,
		Blocked: nullable.NewNullNullable[string]()}
	if c.Blocked != "" {
		out.Blocked = nullable.NewNullableWithValue(c.Blocked)
	}
	return out, nil
}

// BinRecord handles POST /v1/bin.
func (h *HTTP) BinRecord(ctx context.Context, req api.BinRecordRequestObject) (api.BinRecordResponseObject, error) {
	moveTo := ""
	if req.Body.MoveTo != nil {
		moveTo = *req.Body.MoveTo
	}
	e, err := h.svc.Bin(ctx, authz.MustFromContext(ctx), string(req.Body.Kind), req.Body.Id, moveTo)
	if err != nil {
		return nil, err
	}
	return api.BinRecord201JSONResponse(entryToAPI(e)), nil
}

// RestoreBinEntry handles POST /v1/bin/{entryId}/restore.
func (h *HTTP) RestoreBinEntry(ctx context.Context, req api.RestoreBinEntryRequestObject) (api.RestoreBinEntryResponseObject, error) {
	if err := h.svc.Restore(ctx, authz.MustFromContext(ctx), req.EntryId); err != nil {
		return nil, err
	}
	return api.RestoreBinEntry204Response{}, nil
}
