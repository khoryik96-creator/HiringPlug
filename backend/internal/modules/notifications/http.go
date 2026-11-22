package notifications

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the notifications operations of the contract.
type HTTP struct{ svc *Service }

// ListNotifications handles GET /v1/notifications.
func (h *HTTP) ListNotifications(ctx context.Context, _ api.ListNotificationsRequestObject) (api.ListNotificationsResponseObject, error) {
	list, err := h.svc.List(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListNotifications200JSONResponse{Items: make([]api.Notification, len(list.Items)), Unseen: list.Unseen,
		MorningEmail: list.MorningEmail, MutedKinds: kindsAPI(list.Muted)}
	for i, it := range list.Items {
		out.Items[i] = api.Notification{Id: it.ID, Kind: api.NotificationKind(it.Kind), Title: it.Title, Detail: it.Detail, Href: it.Href, At: it.At, New: it.New}
	}
	return out, nil
}

// MarkNotificationsSeen handles POST /v1/notifications/seen.
func (h *HTTP) MarkNotificationsSeen(ctx context.Context, _ api.MarkNotificationsSeenRequestObject) (api.MarkNotificationsSeenResponseObject, error) {
	if err := h.svc.MarkSeen(ctx, authz.MustFromContext(ctx)); err != nil {
		return nil, err
	}
	return api.MarkNotificationsSeen204Response{}, nil
}

// PutNotificationSettings handles PUT /v1/notifications/settings.
func (h *HTTP) PutNotificationSettings(ctx context.Context, req api.PutNotificationSettingsRequestObject) (api.PutNotificationSettingsResponseObject, error) {
	muted := make([]string, len(req.Body.MutedKinds))
	for i, k := range req.Body.MutedKinds {
		muted[i] = string(k)
	}
	saved, err := h.svc.PutPrefs(ctx, authz.MustFromContext(ctx), Prefs{MorningEmail: req.Body.MorningEmail, Muted: muted})
	if err != nil {
		return nil, err
	}
	return api.PutNotificationSettings200JSONResponse{MorningEmail: saved.MorningEmail, MutedKinds: kindsAPI(saved.Muted)}, nil
}

func kindsAPI(kinds []string) []api.NotificationKind {
	out := make([]api.NotificationKind, len(kinds))
	for i, k := range kinds {
		out[i] = api.NotificationKind(k)
	}
	return out
}
