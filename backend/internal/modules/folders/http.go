package folders

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the folders operations of the contract.
type HTTP struct{ svc *Service }

// ListFolders handles GET /v1/folders.
func (h *HTTP) ListFolders(ctx context.Context, req api.ListFoldersRequestObject) (api.ListFoldersResponseObject, error) {
	items, err := h.svc.List(ctx, authz.MustFromContext(ctx), string(req.Params.Kind))
	if err != nil {
		return nil, err
	}
	out := api.ListFolders200JSONResponse{Items: make([]api.Folder, len(items))}
	for i, item := range items {
		out.Items[i] = toAPI(item)
	}
	return out, nil
}

// CreateFolder handles POST /v1/folders.
func (h *HTTP) CreateFolder(ctx context.Context, req api.CreateFolderRequestObject) (api.CreateFolderResponseObject, error) {
	f, err := h.svc.Create(ctx, authz.MustFromContext(ctx), string(req.Body.Kind), req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.CreateFolder201JSONResponse(toAPI(f)), nil
}

// RenameFolder handles PATCH /v1/folders/{folderId}.
func (h *HTTP) RenameFolder(ctx context.Context, req api.RenameFolderRequestObject) (api.RenameFolderResponseObject, error) {
	f, err := h.svc.Rename(ctx, authz.MustFromContext(ctx), req.FolderId, req.Body.Name, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.RenameFolder200JSONResponse(toAPI(f)), nil
}

// DeleteFolder handles DELETE /v1/folders/{folderId}.
func (h *HTTP) DeleteFolder(ctx context.Context, req api.DeleteFolderRequestObject) (api.DeleteFolderResponseObject, error) {
	if err := h.svc.Delete(ctx, authz.MustFromContext(ctx), req.FolderId); err != nil {
		return nil, err
	}
	return api.DeleteFolder204Response{}, nil
}

// ShareFolder handles PUT /v1/folders/{folderId}/shares/{userId}.
func (h *HTTP) ShareFolder(ctx context.Context, req api.ShareFolderRequestObject) (api.ShareFolderResponseObject, error) {
	f, err := h.svc.Share(ctx, authz.MustFromContext(ctx), req.FolderId, req.UserId)
	if err != nil {
		return nil, err
	}
	return api.ShareFolder200JSONResponse(toAPI(f)), nil
}

// UnshareFolder handles DELETE /v1/folders/{folderId}/shares/{userId}.
func (h *HTTP) UnshareFolder(ctx context.Context, req api.UnshareFolderRequestObject) (api.UnshareFolderResponseObject, error) {
	f, err := h.svc.Unshare(ctx, authz.MustFromContext(ctx), req.FolderId, req.UserId)
	if err != nil {
		return nil, err
	}
	out := api.UnshareFolder200JSONResponse{}
	out.Folder.SetNull()
	if f != nil {
		out.Folder.Set(toAPI(*f))
	}
	return out, nil
}

// AddFolderItems handles POST /v1/folders/{folderId}/items.
func (h *HTTP) AddFolderItems(ctx context.Context, req api.AddFolderItemsRequestObject) (api.AddFolderItemsResponseObject, error) {
	f, err := h.svc.AddItems(ctx, authz.MustFromContext(ctx), req.FolderId, req.Body.ItemIds)
	if err != nil {
		return nil, err
	}
	return api.AddFolderItems200JSONResponse(toAPI(f)), nil
}

// RemoveFolderItems handles POST /v1/folders/{folderId}/items/remove.
func (h *HTTP) RemoveFolderItems(ctx context.Context, req api.RemoveFolderItemsRequestObject) (api.RemoveFolderItemsResponseObject, error) {
	f, err := h.svc.RemoveItems(ctx, authz.MustFromContext(ctx), req.FolderId, req.Body.ItemIds)
	if err != nil {
		return nil, err
	}
	return api.RemoveFolderItems200JSONResponse(toAPI(f)), nil
}

// FolderMemberships handles POST /v1/folders/memberships.
func (h *HTTP) FolderMemberships(ctx context.Context, req api.FolderMembershipsRequestObject) (api.FolderMembershipsResponseObject, error) {
	ms, err := h.svc.Memberships(ctx, authz.MustFromContext(ctx), string(req.Body.Kind), req.Body.ItemIds)
	if err != nil {
		return nil, err
	}
	out := api.FolderMemberships200JSONResponse{}
	out.Items = make([]struct {
		FolderId string   `json:"folderId"`
		ItemIds  []string `json:"itemIds"`
	}, len(ms))
	for i, m := range ms {
		out.Items[i].FolderId, out.Items[i].ItemIds = m.FolderID, m.ItemIDs
	}
	return out, nil
}

func toAPI(f Folder) api.Folder {
	shared := f.SharedWith
	if shared == nil {
		shared = []string{}
	}
	return api.Folder{Id: f.ID, Kind: api.FolderKind(f.Kind), Name: f.Name, OwnerId: f.OwnerID, Mine: f.Mine, SharedWith: shared,
		ItemCount: f.ItemCount, Version: f.Version, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt}
}
