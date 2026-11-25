package settings

// Settings > System: the server's paid-AI switch (platform/aipaid). Every
// company's owners and admins see whether paid AI is paused on the server;
// only an owner of the agency that runs it may change it (it stops every
// company's AI). Audited.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/aipaid"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// PaidAI is the switch as one person sees it.
type PaidAI struct {
	Paused    bool // the switch (HP_AI_PAID=off is ByServer)
	ByServer  bool
	Reason    *string
	ChangedAt *time.Time
	CanChange bool
}

// PaidAIOffByServer tells Settings that HP_AI_PAID=off (paused whatever the switch says).
func (m *Module) PaidAIOffByServer(off bool) { m.service.paidOff = off }

// runsServer reports whether p's agency runs this server: HP_BACKUP_TENANT,
// or the only agency on a server with just one.
func (s *Service) runsServer(ctx context.Context, tx pgx.Tx, p authz.Principal) (bool, error) {
	if s.backups.OperatorTenant != "" {
		return p.TenantID == s.backups.OperatorTenant, nil
	}
	var n int64
	err := tx.QueryRow(ctx, `SELECT app_tenant_count()`).Scan(&n)
	return n == 1, err
}

func (s *Service) paidAI(ctx context.Context, tx pgx.Tx, p authz.Principal) (PaidAI, error) {
	sw, err := aipaid.Read(ctx, tx)
	if err != nil {
		return PaidAI{}, err
	}
	out := PaidAI{Paused: sw.Paused, ByServer: s.paidOff, Reason: sw.Reason, ChangedAt: sw.ChangedAt}
	if p.Role == api.RoleOwner {
		if out.CanChange, err = s.runsServer(ctx, tx, p); err != nil {
			return out, err
		}
	}
	return out, nil
}

// SetPaidAI pauses paid AI on the whole server, or turns it back on.
func (s *Service) SetPaidAI(ctx context.Context, p authz.Principal, paused bool, reason string) (SystemStatus, error) {
	var out SystemStatus
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		sw, err := s.paidAI(ctx, tx, p)
		if err != nil {
			return err
		}
		if !sw.CanChange {
			return apperr.New(api.ErrorCodeFORBIDDEN, "Paid AI is paused or turned back on by an owner of the agency that runs this server.")
		}
		reason = strings.TrimSpace(reason)
		if paused && reason == "" {
			reason = "Paused in Settings > System by " + p.Name + "."
		}
		if !paused {
			reason = ""
		}
		if err := aipaid.Set(ctx, tx, paused, reason, p.UserID); err != nil {
			return err
		}
		action := "settings.paid_ai_resumed"
		if paused {
			action = "settings.paid_ai_paused"
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "server", EntityID: "paid_ai",
			Changes: map[string]any{"paused": paused, "reason": reason}}); err != nil {
			return err
		}
		out, err = s.systemStatus(ctx, tx, p)
		return err
	})
	return out, err
}

// SetPaidAi handles PUT /v1/settings/system/paid-ai.
func (h *HTTP) SetPaidAi(ctx context.Context, req api.SetPaidAiRequestObject) (api.SetPaidAiResponseObject, error) {
	reason := ""
	if req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	st, err := h.svc.SetPaidAI(ctx, authz.MustFromContext(ctx), req.Body.Paused, reason)
	if err != nil {
		return nil, err
	}
	return api.SetPaidAi200JSONResponse(systemToAPI(st)), nil
}
