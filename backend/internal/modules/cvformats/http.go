package cvformats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"slices"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the cvformats operations of the contract.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// ListCvTemplates handles GET /v1/cv-templates.
func (h *HTTP) ListCvTemplates(ctx context.Context, _ api.ListCvTemplatesRequestObject) (api.ListCvTemplatesResponseObject, error) {
	views, err := h.svc.Templates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListCvTemplates200JSONResponse{Items: make([]api.CvTemplate, len(views)), Placeholders: Placeholders, Blocks: []string{}}
	for i, v := range views {
		out.Items[i] = api.CvTemplate{Id: v.ID, Name: v.Name, BuiltIn: v.BuiltIn, UploadedByName: c.OrNull(v.UploadedByName), CreatedAt: c.OrNull(v.CreatedAt)}
	}
	for block, fields := range BlockFields {
		out.Blocks = append(out.Blocks, block+": "+strings.Join(fields, ", "))
	}
	slices.Sort(out.Blocks)
	return out, nil
}

// UploadCvTemplate handles POST /v1/cv-templates (multipart: name, file).
func (h *HTTP) UploadCvTemplate(ctx context.Context, req api.UploadCvTemplateRequestObject) (api.UploadCvTemplateResponseObject, error) {
	var name string
	var data []byte
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data upload"}})
		}
		switch part.FormName() {
		case "name":
			b, err := io.ReadAll(io.LimitReader(part, 400))
			if err != nil {
				return nil, err
			}
			name = string(b)
		case "file":
			data, err = io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1))
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > h.maxUploadBytes {
				return nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, fmt.Sprintf("Files can be at most %d MB.", h.maxUploadBytes>>20))
			}
		}
	}
	if len(data) == 0 {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "is required"}})
	}
	v, err := h.svc.Upload(ctx, authz.MustFromContext(ctx), name, data)
	if err != nil {
		return nil, err
	}
	return api.UploadCvTemplate201JSONResponse{Id: v.ID, Name: v.Name, BuiltIn: false, UploadedByName: c.OrNull(v.UploadedByName), CreatedAt: c.OrNull(v.CreatedAt)}, nil
}

// DeleteCvTemplate handles DELETE /v1/cv-templates/{cvTemplateId}.
func (h *HTTP) DeleteCvTemplate(ctx context.Context, req api.DeleteCvTemplateRequestObject) (api.DeleteCvTemplateResponseObject, error) {
	if req.CvTemplateId == StandardID {
		return nil, apperr.NotFound("CV template") // the built-in one cannot be removed
	}
	if err := h.svc.Delete(ctx, authz.MustFromContext(ctx), req.CvTemplateId); err != nil {
		return nil, err
	}
	return api.DeleteCvTemplate204Response{}, nil
}

// DownloadCvTemplate handles GET /v1/cv-templates/{cvTemplateId}/file.
func (h *HTTP) DownloadCvTemplate(ctx context.Context, req api.DownloadCvTemplateRequestObject) (api.DownloadCvTemplateResponseObject, error) {
	data, name, err := h.svc.TemplateFile(ctx, authz.MustFromContext(ctx), req.CvTemplateId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return api.DownloadCvTemplate200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(data), ContentLength: int64(len(data)),
		Headers: api.DownloadCvTemplate200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

// FormatCv handles POST /v1/candidates/{candidateId}/formatted-cvs.
func (h *HTTP) FormatCv(ctx context.Context, req api.FormatCvRequestObject) (api.FormatCvResponseObject, error) {
	b := req.Body
	o := Options{ApplicationID: b.ApplicationId, ShowContact: b.ShowContact != nil && *b.ShowContact, IncludeSalary: b.IncludeSalary != nil && *b.IncludeSalary}
	if b.TemplateId != nil {
		o.TemplateID = *b.TemplateId
	}
	if b.Summary != nil {
		o.Summary = *b.Summary
	}
	id, name, err := h.svc.Format(ctx, authz.MustFromContext(ctx), req.CandidateId, o)
	if err != nil {
		return nil, err
	}
	return api.FormatCv201JSONResponse{DocumentId: id, FileName: name}, nil
}
