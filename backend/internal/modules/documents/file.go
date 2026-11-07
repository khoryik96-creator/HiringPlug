package documents

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// File opens a document's stored file. The row is read in the user's tenant
// (row-level security), so only documents they may see can be opened.
func (s *Service) File(ctx context.Context, p authz.Principal, id string) (Document, io.ReadCloser, int64, error) {
	var row store.Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		return err
	})
	if err != nil {
		return Document{}, nil, 0, err
	}
	f, err := s.files.Open(row.StorageKey)
	if errors.Is(err, os.ErrNotExist) {
		return Document{}, nil, 0, apperr.NotFound("document file")
	}
	if err != nil {
		return Document{}, nil, 0, err
	}
	return fromRow(row), f, row.SizeBytes, nil
}

// GetDocumentFile handles GET /v1/documents/{documentId}/file.
func (h *HTTP) GetDocumentFile(ctx context.Context, req api.GetDocumentFileRequestObject) (api.GetDocumentFileResponseObject, error) {
	doc, f, size, err := h.svc.File(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	// mime encodes non-ASCII names (filename*=utf-8''...), and quotes the rest.
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": doc.FileName})
	return api.GetDocumentFile200ApplicationoctetStreamResponse{
		Body: f, ContentLength: size,
		Headers: api.GetDocumentFile200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

// CVFile reads a candidate's CV to attach to an email, in the caller's
// transaction (row-level security: only documents the user may see). With
// asPDF, a formatted CV's PDF (NOT_FOUND until it is made).
func (m *Module) CVFile(ctx context.Context, tx pgx.Tx, id string, asPDF bool) (Document, []byte, error) {
	row, err := store.New(tx).GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return Document{}, nil, apperr.NotFound("document")
	}
	if err != nil {
		return Document{}, nil, err
	}
	key := row.StorageKey
	doc := fromRow(row)
	if asPDF {
		if row.PdfKey == nil {
			return Document{}, nil, apperr.NotFound("PDF of this CV (make it first)")
		}
		key = *row.PdfKey
		doc.FileName = strings.TrimSuffix(doc.FileName, filepath.Ext(doc.FileName)) + ".pdf"
		doc.ContentType = "application/pdf"
	}
	f, err := m.service.files.Open(key)
	if errors.Is(err, os.ErrNotExist) {
		return Document{}, nil, apperr.NotFound("document file")
	}
	if err != nil {
		return Document{}, nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	return doc, data, err
}

// SourceCV returns one of a candidate's documents and a signed link to its
// file for a worker (the AI worker formatting it), in the caller's
// transaction (row-level security: only documents the user may see).
func (m *Module) SourceCV(ctx context.Context, tx pgx.Tx, id string) (Document, string, error) {
	row, err := store.New(tx).GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return Document{}, "", apperr.NotFound("document")
	}
	if err != nil {
		return Document{}, "", err
	}
	return fromRow(row), m.service.files.SignedURL(row.StorageKey, downloadLinkTTL), nil
}
