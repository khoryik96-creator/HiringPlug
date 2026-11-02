package notes

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the notes operations of the contract.
type HTTP struct{ svc *Service }

// ListNotes handles GET /v1/notes.
func (h *HTTP) ListNotes(ctx context.Context, req api.ListNotesRequestObject) (api.ListNotesResponseObject, error) {
	notes, hidden, err := h.svc.List(ctx, authz.MustFromContext(ctx), string(req.Params.SubjectType), req.Params.SubjectId)
	if err != nil {
		return nil, err
	}
	out := api.ListNotes200JSONResponse{Items: make([]api.Note, len(notes)), HiddenSensitive: hidden}
	for i, n := range notes {
		out.Items[i] = toAPI(n)
	}
	return out, nil
}

// CreateNote handles POST /v1/notes.
func (h *HTTP) CreateNote(ctx context.Context, req api.CreateNoteRequestObject) (api.CreateNoteResponseObject, error) {
	b := req.Body
	d := Draft{SubjectType: string(b.SubjectType), SubjectID: b.SubjectId, Body: b.Body}
	if b.Kind != nil {
		d.Kind = string(*b.Kind)
	}
	if b.Sensitive != nil {
		d.Sensitive = *b.Sensitive
	}
	n, err := h.svc.Create(ctx, authz.MustFromContext(ctx), d)
	if err != nil {
		return nil, err
	}
	return api.CreateNote201JSONResponse(toAPI(n)), nil
}

// DeleteNote handles DELETE /v1/notes/{noteId}.
func (h *HTTP) DeleteNote(ctx context.Context, req api.DeleteNoteRequestObject) (api.DeleteNoteResponseObject, error) {
	if err := h.svc.Delete(ctx, authz.MustFromContext(ctx), req.NoteId); err != nil {
		return nil, err
	}
	return api.DeleteNote204Response{}, nil
}

// ListActivity handles GET /v1/activity.
func (h *HTTP) ListActivity(ctx context.Context, req api.ListActivityRequestObject) (api.ListActivityResponseObject, error) {
	entries, err := h.svc.Activity(ctx, authz.MustFromContext(ctx), string(req.Params.EntityType), req.Params.EntityId)
	if err != nil {
		return nil, err
	}
	out := api.ListActivity200JSONResponse{Items: make([]api.ActivityEntry, len(entries))}
	for i, e := range entries {
		out.Items[i] = api.ActivityEntry{Id: e.ID, Action: e.Action, ActorName: e.ActorName, Changes: e.Changes, At: e.At}
	}
	return out, nil
}

func toAPI(n Note) api.Note {
	return api.Note{
		Id: n.ID, SubjectType: api.NoteSubjectType(n.SubjectType), SubjectId: n.SubjectID, Kind: api.NoteKind(n.Kind),
		Body: n.Body, Sensitive: n.Sensitive, AuthorId: n.AuthorID, AuthorName: n.AuthorName, CreatedAt: n.CreatedAt,
	}
}
