package companies

// HTTP handlers for documents kept with a client's terms and benefits.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListCompanyDocuments handles GET /v1/companies/{companyId}/documents.
func (h *HTTP) ListCompanyDocuments(ctx context.Context, req api.ListCompanyDocumentsRequestObject) (api.ListCompanyDocumentsResponseObject, error) {
	docs, err := h.svc.Documents(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	out := api.CompanyDocumentList{Items: make([]api.CompanyDocument, len(docs))}
	for i, d := range docs {
		out.Items[i] = documentToAPI(d)
	}
	return api.ListCompanyDocuments200JSONResponse(out), nil
}

// AddCompanyDocument handles POST /v1/companies/{companyId}/documents.
func (h *HTTP) AddCompanyDocument(ctx context.Context, req api.AddCompanyDocumentRequestObject) (api.AddCompanyDocumentResponseObject, error) {
	section, name, data, err := readDocumentForm(req.Body, h.maxUploadBytes)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.AddDocument(ctx, authz.MustFromContext(ctx), req.CompanyId, section, name, data)
	if err != nil {
		return nil, err
	}
	return api.AddCompanyDocument201JSONResponse(documentToAPI(d)), nil
}

// DeleteCompanyDocument handles DELETE /v1/companies/{companyId}/documents/{documentId}.
func (h *HTTP) DeleteCompanyDocument(ctx context.Context, req api.DeleteCompanyDocumentRequestObject) (api.DeleteCompanyDocumentResponseObject, error) {
	if err := h.svc.DeleteDocument(ctx, authz.MustFromContext(ctx), req.CompanyId, req.DocumentId); err != nil {
		return nil, err
	}
	return api.DeleteCompanyDocument204Response{}, nil
}

// GetCompanyDocumentFile handles GET /v1/companies/{companyId}/documents/{documentId}/file.
func (h *HTTP) GetCompanyDocumentFile(ctx context.Context, req api.GetCompanyDocumentFileRequestObject) (api.GetCompanyDocumentFileResponseObject, error) {
	f, err := h.svc.DocumentFile(ctx, authz.MustFromContext(ctx), req.CompanyId, req.DocumentId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})
	return api.GetCompanyDocumentFile200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(f.Data), ContentLength: int64(len(f.Data)),
		Headers: api.GetCompanyDocumentFile200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func documentToAPI(d Document) api.CompanyDocument {
	return api.CompanyDocument{
		Id: d.ID, Section: api.CompanyDocumentSection(d.Section), FileName: d.FileName, ContentType: d.ContentType,
		SizeBytes: int(d.SizeBytes), CreatedAt: d.CreatedAt, CreatedByName: c.OrNull(d.CreatedByName),
	}
}

// readDocumentForm reads the upload form: its section and file.
func readDocumentForm(r *multipart.Reader, limit int64) (section, name string, data []byte, err error) {
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", nil, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data form"}})
		}
		switch part.FormName() {
		case "section":
			b, err := io.ReadAll(io.LimitReader(part, 32))
			if err != nil {
				return "", "", nil, apperr.Validation(apperr.FieldErrors{{Field: "section", Message: "could not be read"}})
			}
			section = string(b)
		case "file":
			name = part.FileName()
			if data, err = io.ReadAll(io.LimitReader(part, limit+1)); err != nil {
				return "", "", nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "could not be read"}})
			}
			if int64(len(data)) > limit {
				return "", "", nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, "The file is too large.")
			}
		default:
			return "", "", nil, apperr.Validation(apperr.FieldErrors{{Field: part.FormName(), Message: "unexpected field"}})
		}
	}
	return section, name, data, nil
}
