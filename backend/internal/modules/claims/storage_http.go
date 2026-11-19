package claims

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func ptrTo[T any](v T) *T { return &v }

func (h *HTTP) storageOn() (*Storage, error) {
	if h.storage == nil {
		return nil, apperr.New(api.ErrorCodeCLAIMSTORAGENOTCONNECTED, "Claims storage is not set up on this server.")
	}
	return h.storage, nil
}

// GetClaimStorage handles GET /v1/claims/storage.
func (h *HTTP) GetClaimStorage(ctx context.Context, _ api.GetClaimStorageRequestObject) (api.GetClaimStorageResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	v, err := s.View(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetClaimStorage200JSONResponse(v), nil
}

// UpdateClaimStorage handles PUT /v1/claims/storage.
func (h *HTTP) UpdateClaimStorage(ctx context.Context, r api.UpdateClaimStorageRequestObject) (api.UpdateClaimStorageResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	v, err := s.SetFolder(ctx, authz.MustFromContext(ctx), r.Body.RootFolder, r.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.UpdateClaimStorage200JSONResponse(v), nil
}

// DisconnectClaimStorage handles DELETE /v1/claims/storage.
func (h *HTTP) DisconnectClaimStorage(ctx context.Context, _ api.DisconnectClaimStorageRequestObject) (api.DisconnectClaimStorageResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	if err := s.Disconnect(ctx, authz.MustFromContext(ctx)); err != nil {
		return nil, err
	}
	return api.DisconnectClaimStorage204Response{}, nil
}

// AuthorizeClaimStorage handles POST /v1/claims/storage/authorize.
func (h *HTTP) AuthorizeClaimStorage(ctx context.Context, r api.AuthorizeClaimStorageRequestObject) (api.AuthorizeClaimStorageResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	target, err := s.Authorize(ctx, authz.MustFromContext(ctx), string(r.Body.Provider))
	if err != nil {
		return nil, err
	}
	return api.AuthorizeClaimStorage200JSONResponse{Url: target}, nil
}

// ClaimStorageCallback handles GET /v1/claims/storage/callback (public): the
// browser goes back to Settings, which finishes with the ticket.
func (h *HTTP) ClaimStorageCallback(ctx context.Context, r api.ClaimStorageCallbackRequestObject) (api.ClaimStorageCallbackResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	outcome := "failed"
	if ticket, err := s.Park(ctx, deref(r.Params.State), deref(r.Params.Code), deref(r.Params.Error)); err == nil {
		outcome = "confirm&ticket=" + ticket
	}
	target := s.config.PublicURL + "/#/settings/claims-storage?storage=" + outcome
	cache, referrer := "no-store", "no-referrer"
	return api.ClaimStorageCallback303Response{Headers: api.ClaimStorageCallback303ResponseHeaders{Location: &target, CacheControl: &cache, ReferrerPolicy: &referrer}}, nil
}

// CompleteClaimStorage handles POST /v1/claims/storage/complete.
func (h *HTTP) CompleteClaimStorage(ctx context.Context, r api.CompleteClaimStorageRequestObject) (api.CompleteClaimStorageResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	v, err := s.Complete(ctx, authz.MustFromContext(ctx), r.Body.Ticket)
	if err != nil {
		return nil, err
	}
	return api.CompleteClaimStorage200JSONResponse(v), nil
}

// RetryClaimUploads handles POST /v1/claims/storage/retry.
func (h *HTTP) RetryClaimUploads(ctx context.Context, _ api.RetryClaimUploadsRequestObject) (api.RetryClaimUploadsResponseObject, error) {
	s, err := h.storageOn()
	if err != nil {
		return nil, err
	}
	n, err := s.Retry(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.RetryClaimUploads200JSONResponse{Queued: n}, nil
}
