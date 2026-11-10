package intake

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// ListCvImports handles GET /v1/cv-imports.
func (h *HTTP) ListCvImports(ctx context.Context, _ api.ListCvImportsRequestObject) (api.ListCvImportsResponseObject, error) {
	all, err := h.svc.List(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.CvImportList{Items: make([]api.CvImport, len(all))}
	for i, imp := range all {
		out.Items[i] = importToAPI(imp)
	}
	return api.ListCvImports200JSONResponse(out), nil
}

// CreateCvImport handles POST /v1/cv-imports.
func (h *HTTP) CreateCvImport(ctx context.Context, req api.CreateCvImportRequestObject) (api.CreateCvImportResponseObject, error) {
	imp, err := h.svc.Create(ctx, authz.MustFromContext(ctx), req.Body.Source)
	if err != nil {
		return nil, err
	}
	return api.CreateCvImport201JSONResponse(importToAPI(imp)), nil
}

// GetCvImport handles GET /v1/cv-imports/{cvImportId}.
func (h *HTTP) GetCvImport(ctx context.Context, req api.GetCvImportRequestObject) (api.GetCvImportResponseObject, error) {
	imp, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.CvImportId)
	if err != nil {
		return nil, err
	}
	return api.GetCvImport200JSONResponse(importToAPI(imp)), nil
}

// AddCvImportFile handles POST /v1/cv-imports/{cvImportId}/files (multipart, one file).
func (h *HTTP) AddCvImportFile(ctx context.Context, req api.AddCvImportFileRequestObject) (api.AddCvImportFileResponseObject, error) {
	var fileName string
	var data []byte
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data form"}})
		}
		if part.FormName() != "file" {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: part.FormName(), Message: "unexpected field; send only file"}})
		}
		fileName = part.FileName()
		if data, err = io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1)); err != nil {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "could not be read"}})
		}
		if int64(len(data)) > h.maxUploadBytes {
			return nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, fmt.Sprintf("Files can be at most %d MB.", h.maxUploadBytes>>20))
		}
	}
	if len(data) == 0 {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "is required and must not be empty"}})
	}
	if fileName == "" {
		fileName = "CV"
	}
	if r := []rune(fileName); len(r) > 255 {
		fileName = string(r[:255])
	}
	item, err := h.svc.AddFile(ctx, authz.MustFromContext(ctx), req.CvImportId, fileName, data)
	if err != nil {
		return nil, err
	}
	return api.AddCvImportFile201JSONResponse(itemToAPI(item)), nil
}

func itemToAPI(it Item) api.CvImportItem {
	return api.CvImportItem{
		Id: it.ID, FileName: it.FileName, Status: api.CvImportItemStatus(it.Status), CandidateId: c.OrNull(it.CandidateID),
		CandidateName: c.OrNull(it.CandidateName), Error: c.OrNull(it.Error), UpdatedAt: it.UpdatedAt,
	}
}

func importToAPI(imp Import) api.CvImport {
	statuses := make([]string, len(imp.Items))
	items := make([]api.CvImportItem, len(imp.Items))
	for i, it := range imp.Items {
		statuses[i], items[i] = it.Status, itemToAPI(it)
	}
	n := Counts(statuses)
	return api.CvImport{
		Id: imp.ID, Source: imp.Source, CreatedAt: imp.CreatedAt, Items: items,
		Counts: api.CvImportCounts{Reading: n[Reading], Created: n[Created], Added: n[Added], Unread: n[Unread], Failed: n[Failed], Duplicate: n[Duplicate]},
	}
}
