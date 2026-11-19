package claims

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the claims part of the API: contract types <-> service.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

func dayOf(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func filter(status *api.ClaimStatus, userID *string, from, to *openapi_types.Date, limit, offset *int) Filter {
	f := Filter{UserID: userID, From: dayOf(from), To: dayOf(to), Limit: 200}
	if status != nil {
		s := string(*status)
		f.Status = &s
	}
	if limit != nil {
		f.Limit = *limit
	}
	if offset != nil {
		f.Offset = *offset
	}
	return f
}

// ListMyClaims handles GET /v1/claims.
func (h *HTTP) ListMyClaims(ctx context.Context, r api.ListMyClaimsRequestObject) (api.ListMyClaimsResponseObject, error) {
	q := r.Params
	page, err := h.svc.Mine(ctx, authz.MustFromContext(ctx), filter(q.Status, nil, q.From, q.To, q.Limit, q.Offset))
	if err != nil {
		return nil, err
	}
	return api.ListMyClaims200JSONResponse(page), nil
}

// ListTeamClaims handles GET /v1/claims/team.
func (h *HTTP) ListTeamClaims(ctx context.Context, r api.ListTeamClaimsRequestObject) (api.ListTeamClaimsResponseObject, error) {
	q := r.Params
	page, err := h.svc.Team(ctx, authz.MustFromContext(ctx), filter(q.Status, q.UserId, q.From, q.To, q.Limit, q.Offset))
	if err != nil {
		return nil, err
	}
	return api.ListTeamClaims200JSONResponse(page), nil
}

// UploadClaimReceipt handles POST /v1/claims (multipart: file).
func (h *HTTP) UploadClaimReceipt(ctx context.Context, r api.UploadClaimReceiptRequestObject) (api.UploadClaimReceiptResponseObject, error) {
	var fileName string
	var data []byte
	var bad apperr.FieldErrors
	for {
		part, err := r.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, uploadReadError(err)
		}
		if part.FormName() != "file" {
			bad.Add(part.FormName(), "unexpected field; send only the file")
			continue
		}
		fileName = part.FileName()
		data, err = io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1))
		if err != nil {
			return nil, uploadReadError(err)
		}
		if int64(len(data)) > h.maxUploadBytes {
			return nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, fmt.Sprintf("Receipts can be at most %d MB.", h.maxUploadBytes>>20))
		}
	}
	if len(data) == 0 {
		bad.Add("file", "is required and must not be empty")
	}
	if err := bad.Err(); err != nil {
		return nil, err
	}
	claim, err := h.svc.Upload(ctx, authz.MustFromContext(ctx), fileName, data)
	if err != nil {
		return nil, err
	}
	return api.UploadClaimReceipt201JSONResponse(claim), nil
}

func uploadReadError(err error) error {
	if _, ok := apperr.As(err); ok {
		return err
	}
	// Too-large bodies surface as *http.MaxBytesError, which WriteError maps
	// to 413; anything else is a malformed multipart body.
	if strings.Contains(err.Error(), "request body too large") {
		return err
	}
	return apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data upload"}})
}

// GetMyClaimSummary handles GET /v1/claims/summary.
func (h *HTTP) GetMyClaimSummary(ctx context.Context, r api.GetMyClaimSummaryRequestObject) (api.GetMyClaimSummaryResponseObject, error) {
	out, err := h.svc.Summary(ctx, authz.MustFromContext(ctx), false, dayOf(r.Params.From), dayOf(r.Params.To))
	if err != nil {
		return nil, err
	}
	return api.GetMyClaimSummary200JSONResponse(out), nil
}

// GetTeamClaimSummary handles GET /v1/claims/team/summary.
func (h *HTTP) GetTeamClaimSummary(ctx context.Context, r api.GetTeamClaimSummaryRequestObject) (api.GetTeamClaimSummaryResponseObject, error) {
	out, err := h.svc.Summary(ctx, authz.MustFromContext(ctx), true, dayOf(r.Params.From), dayOf(r.Params.To))
	if err != nil {
		return nil, err
	}
	return api.GetTeamClaimSummary200JSONResponse(out), nil
}

func refs(in []api.ClaimRef) []Ref {
	out := make([]Ref, 0, len(in))
	for _, r := range in {
		out = append(out, Ref{ID: r.Id, Version: r.Version})
	}
	return out
}

// SubmitClaims handles POST /v1/claims/submit.
func (h *HTTP) SubmitClaims(ctx context.Context, r api.SubmitClaimsRequestObject) (api.SubmitClaimsResponseObject, error) {
	items, err := h.svc.Submit(ctx, authz.MustFromContext(ctx), refs(r.Body.Claims))
	if err != nil {
		return nil, err
	}
	return api.SubmitClaims200JSONResponse(api.ClaimList{Items: items}), nil
}

// ReviewClaims handles POST /v1/claims/review.
func (h *HTTP) ReviewClaims(ctx context.Context, r api.ReviewClaimsRequestObject) (api.ReviewClaimsResponseObject, error) {
	items, err := h.svc.Review(ctx, authz.MustFromContext(ctx), refs(r.Body.Claims), string(r.Body.Decision), r.Body.Note)
	if err != nil {
		return nil, err
	}
	return api.ReviewClaims200JSONResponse(api.ClaimList{Items: items}), nil
}

// GetClaim handles GET /v1/claims/{claimId}.
func (h *HTTP) GetClaim(ctx context.Context, r api.GetClaimRequestObject) (api.GetClaimResponseObject, error) {
	claim, err := h.svc.Get(ctx, authz.MustFromContext(ctx), r.ClaimId)
	if err != nil {
		return nil, err
	}
	return api.GetClaim200JSONResponse(claim), nil
}

// change reads a PATCH body: a field left out stays, null clears it.
func change(in api.ClaimUpdate) Change {
	c := Change{Version: in.Version, Currency: in.Currency}
	str := func(field string, v nullable.Nullable[string], to **string) {
		if !v.IsSpecified() {
			return
		}
		if v.IsNull() {
			c.Clear = append(c.Clear, field)
			return
		}
		s := v.MustGet()
		*to = &s
	}
	str(FieldMerchant, in.Merchant, &c.Merchant)
	str(FieldReceiptType, in.ReceiptType, &c.ReceiptType)
	str(FieldClaimType, in.ClaimType, &c.ClaimType)
	str(FieldPurpose, in.Purpose, &c.Purpose)
	str(FieldAmount, in.Amount, &c.Amount)
	if in.ReceiptDate.IsSpecified() {
		if in.ReceiptDate.IsNull() {
			c.Clear = append(c.Clear, FieldReceiptDate)
		} else {
			d := in.ReceiptDate.MustGet().Time
			c.ReceiptDate = &d
		}
	}
	if in.Link.IsSpecified() {
		if in.Link.IsNull() {
			c.Clear = append(c.Clear, FieldLink)
		} else {
			l := in.Link.MustGet()
			kind, id := string(l.Kind), l.Id
			c.LinkKind, c.LinkID = &kind, &id
		}
	}
	return c
}

// UpdateClaim handles PATCH /v1/claims/{claimId}.
func (h *HTTP) UpdateClaim(ctx context.Context, r api.UpdateClaimRequestObject) (api.UpdateClaimResponseObject, error) {
	claim, err := h.svc.Update(ctx, authz.MustFromContext(ctx), r.ClaimId, change(*r.Body))
	if err != nil {
		return nil, err
	}
	return api.UpdateClaim200JSONResponse(claim), nil
}

// DeleteClaim handles DELETE /v1/claims/{claimId}.
func (h *HTTP) DeleteClaim(ctx context.Context, r api.DeleteClaimRequestObject) (api.DeleteClaimResponseObject, error) {
	if err := h.svc.Delete(ctx, authz.MustFromContext(ctx), r.ClaimId); err != nil {
		return nil, err
	}
	return api.DeleteClaim204Response{}, nil
}

// WithdrawClaim handles POST /v1/claims/{claimId}/withdraw.
func (h *HTTP) WithdrawClaim(ctx context.Context, r api.WithdrawClaimRequestObject) (api.WithdrawClaimResponseObject, error) {
	claim, err := h.svc.Withdraw(ctx, authz.MustFromContext(ctx), r.ClaimId, r.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.WithdrawClaim200JSONResponse(claim), nil
}

// GetClaimReceipt handles GET /v1/claims/{claimId}/receipt.
func (h *HTTP) GetClaimReceipt(ctx context.Context, r api.GetClaimReceiptRequestObject) (api.GetClaimReceiptResponseObject, error) {
	name, f, size, err := h.svc.Receipt(ctx, authz.MustFromContext(ctx), r.ClaimId)
	if err != nil {
		return nil, err
	}
	// mime encodes non-ASCII names (filename*=utf-8''...), and quotes the rest.
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return api.GetClaimReceipt200ApplicationoctetStreamResponse{
		Body: f, ContentLength: size,
		Headers: api.GetClaimReceipt200ResponseHeaders{ContentDisposition: disposition, CacheControl: "private, no-store"},
	}, nil
}
