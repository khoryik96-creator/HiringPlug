package contacts

import (
	"context"
	"net/http"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func (h *HTTP) GetApolloConnection(ctx context.Context, _ api.GetApolloConnectionRequestObject) (api.GetApolloConnectionResponseObject, error) {
	row, err := h.svc.readApolloConnection(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return noStoreApolloConnection{api.GetApolloConnection200JSONResponse(h.svc.apolloConnection(row))}, nil
}

func (h *HTTP) UpdateApolloConnection(ctx context.Context, req api.UpdateApolloConnectionRequestObject) (api.UpdateApolloConnectionResponseObject, error) {
	p := authz.MustFromContext(ctx)
	var out api.ApolloConnection
	var err error
	switch req.Body.Action {
	case api.ApolloConnectionActionActionStart:
		out, err = h.svc.startApollo(ctx, p)
	case api.ApolloConnectionActionActionPoll:
		out, err = h.svc.pollApollo(ctx, p)
	case api.ApolloConnectionActionActionDisconnect:
		out, err = h.svc.disconnectApollo(ctx, p)
	default:
		err = apperr.Validation(apperr.FieldErrors{{Field: "action", Message: "choose a sign-in action"}})
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateApolloConnection200JSONResponse(out), nil
}

func (h *HTTP) LookupApolloWorkEmail(ctx context.Context, req api.LookupApolloWorkEmailRequestObject) (api.LookupApolloWorkEmailResponseObject, error) {
	out, err := h.svc.lookupApollo(ctx, authz.MustFromContext(ctx), req.CompanyId, *req.Body)
	if err != nil {
		return nil, err
	}
	return api.LookupApolloWorkEmail200JSONResponse(out), nil
}

// An authorization URL belongs to the requesting browser session.
type noStoreApolloConnection struct {
	api.GetApolloConnection200JSONResponse
}

func (r noStoreApolloConnection) VisitGetApolloConnectionResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "no-store")
	return r.GetApolloConnection200JSONResponse.VisitGetApolloConnectionResponse(w)
}
