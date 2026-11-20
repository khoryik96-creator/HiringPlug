package emails

// Email signatures: each person's own, formatted (text, links, banners),
// added by the web app to the emails they write in Outlook Mail. Outlook
// gives other apps no way to read a person's Outlook signature, so it is
// kept here.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func signatureToAPI(row store.EmailSignature) api.EmailSignature {
	return api.EmailSignature{BodyHtml: row.BodyHtml, Version: int(row.Version), UpdatedAt: apiconv.OrNull(&row.UpdatedAt)}
}

// Signature is the person's signature (empty, version 0, until saved).
func (s *Service) Signature(ctx context.Context, p authz.Principal) (api.EmailSignature, error) {
	out := api.EmailSignature{UpdatedAt: apiconv.OrNull[time.Time](nil)}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetEmailSignature(ctx)
		if database.IsNoRows(err) {
			return nil
		}
		if err == nil {
			out = signatureToAPI(row)
		}
		return err
	})
	return out, err
}

// SaveSignature keeps what a message may hold (formatting, links, the
// company's banners) and nothing else.
func (s *Service) SaveSignature(ctx context.Context, p authz.Principal, html string, version int) (api.EmailSignature, error) {
	clean := SanitizeHTML(html)
	if TextFromHTML(clean) == "" && len(BannerIDs(clean)) == 0 {
		clean = ""
	}
	var out api.EmailSignature
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := s.checkBanners(ctx, q, clean); err != nil {
			return err
		}
		row, err := q.SaveEmailSignature(ctx, store.SaveEmailSignatureParams{BodyHtml: clean, Version: int32(version)}) //nolint:gosec // a version number
		if database.IsNoRows(err) {
			return apperr.StaleVersion("email signature")
		}
		if err != nil {
			return err
		}
		out = signatureToAPI(row)
		// What it says is the person's own: the audit notes only that it changed.
		return audit.Record(ctx, tx, audit.Entry{Action: "email_signature.saved", EntityType: "user", EntityID: p.UserID})
	})
	return out, err
}

// GetEmailSignature handles GET /v1/email-signature.
func (h *HTTP) GetEmailSignature(ctx context.Context, _ api.GetEmailSignatureRequestObject) (api.GetEmailSignatureResponseObject, error) {
	out, err := h.svc.Signature(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetEmailSignature200JSONResponse(out), nil
}

// SaveEmailSignature handles PUT /v1/email-signature.
func (h *HTTP) SaveEmailSignature(ctx context.Context, r api.SaveEmailSignatureRequestObject) (api.SaveEmailSignatureResponseObject, error) {
	out, err := h.svc.SaveSignature(ctx, authz.MustFromContext(ctx), r.Body.BodyHtml, r.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.SaveEmailSignature200JSONResponse(out), nil
}
