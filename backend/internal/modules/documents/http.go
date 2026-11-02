package documents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the documents operations of the contract.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// UploadCandidateDocument handles POST /v1/candidates/{candidateId}/documents
// (multipart/form-data with fields `source` and `file`, in any order).
func (h *HTTP) UploadCandidateDocument(ctx context.Context, req api.UploadCandidateDocumentRequestObject) (api.UploadCandidateDocumentResponseObject, error) {
	var source, fileName string
	var data []byte
	var fields apperr.FieldErrors
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, uploadReadError(err)
		}
		switch part.FormName() {
		case "source":
			b, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				return nil, uploadReadError(err)
			}
			source = strings.TrimSpace(string(b))
		case "file":
			fileName = part.FileName()
			data, err = io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1))
			if err != nil {
				return nil, uploadReadError(err)
			}
			if int64(len(data)) > h.maxUploadBytes {
				return nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE,
					fmt.Sprintf("Files can be at most %d MB.", h.maxUploadBytes>>20))
			}
		default:
			fields.Add(part.FormName(), "unexpected field; send only source and file")
		}
	}
	if !api.DocumentSource(source).Valid() {
		fields.Add("source", "must be one of jobstreet, linkedin, original, formatted, other")
	}
	if len(data) == 0 {
		fields.Add("file", "is required and must not be empty")
	}
	if err := fields.Err(); err != nil {
		return nil, err
	}
	doc, err := h.svc.Upload(ctx, authz.MustFromContext(ctx), req.CandidateId, source, fileName, data)
	if err != nil {
		return nil, err
	}
	return api.UploadCandidateDocument201JSONResponse(documentToAPI(doc, false)), nil
}

// ListCandidateDocuments handles GET /v1/candidates/{candidateId}/documents.
func (h *HTTP) ListCandidateDocuments(ctx context.Context, req api.ListCandidateDocumentsRequestObject) (api.ListCandidateDocumentsResponseObject, error) {
	docs, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ListCandidateDocuments200JSONResponse{Items: make([]api.Document, len(docs))}
	for i, d := range docs {
		out.Items[i] = documentToAPI(d, false)
	}
	return out, nil
}

// GetDocument handles GET /v1/documents/{documentId}.
func (h *HTTP) GetDocument(ctx context.Context, req api.GetDocumentRequestObject) (api.GetDocumentResponseObject, error) {
	doc, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	return api.GetDocument200JSONResponse(documentToAPI(doc, true)), nil
}

// ParseDocument handles POST /v1/documents/{documentId}/parse.
func (h *HTTP) ParseDocument(ctx context.Context, req api.ParseDocumentRequestObject) (api.ParseDocumentResponseObject, error) {
	doc, err := h.svc.Parse(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	return api.ParseDocument202JSONResponse(documentToAPI(doc, false)), nil
}

func documentToAPI(d Document, withText bool) api.Document {
	out := api.Document{
		Id: d.ID, CandidateId: d.CandidateID, Source: api.DocumentSource(d.Source),
		FileName: d.FileName, ContentType: d.ContentType, SizeBytes: d.SizeBytes, Sha256: d.SHA256,
		Status:             api.DocumentStatus(d.Status),
		PageCount:          orNull(d.PageCount),
		Extractor:          orNull(d.Extractor),
		ErrorMessage:       orNull(d.ErrorMessage),
		ExtractionWarnings: d.Warnings,
		CreatedAt:          d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.ParseStatus != nil {
		out.ParseStatus = nullable.NewNullableWithValue(api.DocumentParseStatus(*d.ParseStatus))
	} else {
		out.ParseStatus = nullable.NewNullNullable[api.DocumentParseStatus]()
	}
	if withText {
		out.ExtractedText = orNull(d.ExtractedText)
		out.ParsedProfile = nullable.NewNullNullable[api.ParsedProfile]()
		var profile api.ParsedProfile
		if len(d.ParsedProfile) > 0 && json.Unmarshal(d.ParsedProfile, &profile) == nil {
			out.ParsedProfile = nullable.NewNullableWithValue(profile)
		}
	}
	return out
}

func uploadReadError(err error) error {
	if _, ok := apperr.As(err); ok {
		return err
	}
	// Too-large bodies surface here as *http.MaxBytesError; WriteError maps
	// that to 413. Anything else is a malformed multipart body.
	if strings.Contains(err.Error(), "request body too large") {
		return err
	}
	return apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data upload"}})
}

func orNull[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}
