package auditlog

// HTTP handlers: contract types <-> service calls; no business rules.

import (
	"bytes"
	"context"
	"mime"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

// HTTP implements the module's endpoints.
type HTTP struct{ svc *Service }

// defaultLimit is a page's size when none is asked for.
const defaultLimit = 50

func day(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}

func limitOf(l *int) int {
	if l == nil {
		return defaultLimit
	}
	return *l
}

func entryToAPI(e Entry) api.AuditEntry {
	changes := e.Changes
	if changes == nil {
		changes = map[string]any{}
	}
	return api.AuditEntry{
		Id: e.ID, At: e.CreatedAt, ActorId: e.ActorID, ActorKind: api.AuditActorKind(e.ActorKind), ActorName: e.ActorName,
		Action: e.Action, EntityType: e.EntityType, EntityId: e.EntityID, EntityName: c.OrNull(e.EntityName), Changes: changes,
	}
}

// ListAuditLog handles GET /v1/audit-log.
func (h *HTTP) ListAuditLog(ctx context.Context, req api.ListAuditLogRequestObject) (api.ListAuditLogResponseObject, error) {
	q := req.Params
	page, err := h.svc.Log(ctx, authz.MustFromContext(ctx), LogFilter{
		Actor: q.Actor, Action: q.Action, EntityType: q.EntityType, EntityID: q.EntityId, From: day(q.From), To: day(q.To),
	}, q.Cursor, limitOf(q.Limit))
	if err != nil {
		return nil, err
	}
	out := api.ListAuditLog200JSONResponse{Items: make([]api.AuditEntry, len(page.Items)), NextCursor: c.OrNull(page.NextCursor)}
	for i, e := range page.Items {
		out.Items[i] = entryToAPI(e)
	}
	return out, nil
}

func viewToAPI(v View) api.SalaryView {
	return api.SalaryView{
		Id: v.ID, Day: openapi_types.Date{Time: v.Day}, UserId: v.UserID, UserName: v.UserName, Kind: api.SalaryViewKind(v.Kind),
		CandidateId: c.OrNull(v.CandidateID), CandidateName: c.OrNull(v.CandidateName), Candidates: v.Candidates, Count: v.Count,
		FirstAt: v.FirstAt, LastAt: v.LastAt,
	}
}

// ListSalaryViews handles GET /v1/salary-views.
func (h *HTTP) ListSalaryViews(ctx context.Context, req api.ListSalaryViewsRequestObject) (api.ListSalaryViewsResponseObject, error) {
	q := req.Params
	page, err := h.svc.SalaryViews(ctx, authz.MustFromContext(ctx), ViewFilter{
		User: q.User, CandidateID: q.CandidateId, Kind: c.Convert[string](q.Kind), From: day(q.From), To: day(q.To),
	}, q.Cursor, limitOf(q.Limit))
	if err != nil {
		return nil, err
	}
	out := api.ListSalaryViews200JSONResponse{Items: make([]api.SalaryView, len(page.Items)), NextCursor: c.OrNull(page.NextCursor)}
	for i, v := range page.Items {
		out.Items[i] = viewToAPI(v)
	}
	return out, nil
}

func disposition(f sheet.File) *string {
	d := mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})
	return &d
}

// ExportAuditLog handles POST /v1/audit-log/export.
func (h *HTTP) ExportAuditLog(ctx context.Context, req api.ExportAuditLogRequestObject) (api.ExportAuditLogResponseObject, error) {
	b := req.Body
	file, err := h.svc.ExportLog(ctx, authz.MustFromContext(ctx), LogFilter{
		Actor: b.Actor, Action: b.Action, EntityType: b.EntityType, EntityID: b.EntityId, From: day(b.From), To: day(b.To),
	}, string(b.Format))
	if err != nil {
		return nil, err
	}
	return api.ExportAuditLog200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportAuditLog200ResponseHeaders{ContentDisposition: disposition(file)},
	}, nil
}

// ExportSalaryViews handles POST /v1/salary-views/export.
func (h *HTTP) ExportSalaryViews(ctx context.Context, req api.ExportSalaryViewsRequestObject) (api.ExportSalaryViewsResponseObject, error) {
	b := req.Body
	file, err := h.svc.ExportSalaryViews(ctx, authz.MustFromContext(ctx), ViewFilter{
		User: b.User, CandidateID: b.CandidateId, Kind: c.Convert[string](b.Kind), From: day(b.From), To: day(b.To),
	}, string(b.Format))
	if err != nil {
		return nil, err
	}
	return api.ExportSalaryViews200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportSalaryViews200ResponseHeaders{ContentDisposition: disposition(file)},
	}, nil
}
