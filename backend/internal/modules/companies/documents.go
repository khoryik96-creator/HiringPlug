package companies

// Documents kept with a client's terms of business or benefits (the signed
// terms, a benefits brochure): their files in blob storage, deleted only by
// those given companies.documents:delete.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Document is a document kept with a client's terms or benefits.
type Document struct {
	store.CompanyDocument
	CreatedByName *string
}

// Sections a document is kept in.
var documentSections = map[string]bool{"terms": true, "benefits": true}

// Documents lists a client's documents, oldest first.
func (s *Service) Documents(ctx context.Context, p authz.Principal, companyID string) ([]Document, error) {
	var out []Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := s.mustExist(ctx, q, companyID); err != nil {
			return err
		}
		rows, err := q.ListCompanyDocuments(ctx, companyID)
		if err != nil {
			return err
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.CreatedBy
		}
		names := map[string]string{}
		if s.users != nil && len(ids) > 0 {
			if names, err = s.users.UserNames(ctx, tx, ids); err != nil {
				return err
			}
		}
		out = make([]Document, len(rows))
		for i, r := range rows {
			out[i] = Document{CompanyDocument: r}
			if n, ok := names[r.CreatedBy]; ok {
				out[i].CreatedByName = &n
			}
		}
		return nil
	})
	return out, err
}

// AddDocument keeps a document with a client's terms or benefits. Its type
// is told from its bytes, never from its name.
func (s *Service) AddDocument(ctx context.Context, p authz.Principal, companyID, section, fileName string, data []byte) (Document, error) {
	if !documentSections[section] {
		return Document{}, apperr.Validation(apperr.FieldErrors{{Field: "section", Message: "terms or benefits"}})
	}
	if len(data) == 0 {
		return Document{}, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "is required and must not be empty"}})
	}
	ctype, ok := filekind.Detect(fileName, data)
	if !ok {
		return Document{}, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "upload " + filekind.Accepted}})
	}
	name := strings.TrimSpace(filepath.Base(fileName))
	if name == "" || name == "." {
		name = "document"
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	key := p.TenantID + "/cdoc_" + randomKey()
	if err := s.files.Put(key, data); err != nil {
		return Document{}, err
	}
	var out Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := s.mustExist(ctx, q, companyID); err != nil {
			return err
		}
		row, err := q.InsertCompanyDocument(ctx, store.InsertCompanyDocumentParams{
			CompanyID: companyID, Section: section, FileName: name, ContentType: ctype,
			SizeBytes: int32(len(data)), StorageKey: key, //nolint:gosec // at most the upload limit
		})
		if err != nil {
			return err
		}
		out = Document{CompanyDocument: row, CreatedByName: &p.Name}
		return audit.Record(ctx, tx, audit.Entry{Action: "company.document_added", EntityType: "company", EntityID: companyID,
			Changes: map[string]any{"documentId": row.ID, "section": section, "fileName": name}})
	})
	if err != nil {
		_ = s.files.Delete(key)
	}
	return out, err
}

// DeleteDocument removes a document (companies.documents:delete), its file
// after the transaction commits.
func (s *Service) DeleteDocument(ctx context.Context, p authz.Principal, companyID, id string) error {
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		doc, err := q.GetCompanyDocument(ctx, store.GetCompanyDocumentParams{ID: id, CompanyID: companyID})
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		if key, err = q.DeleteCompanyDocument(ctx, store.DeleteCompanyDocumentParams{ID: id, CompanyID: companyID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "company.document_deleted", EntityType: "company", EntityID: companyID,
			Changes: map[string]any{"documentId": id, "section": doc.Section, "fileName": doc.FileName}})
	})
	if err == nil {
		if err := s.files.Delete(key); err != nil {
			logx.From(ctx).Warn("could not remove a deleted company document's file", "error", err)
		}
	}
	return err
}

// DocumentFile is a document to download.
type DocumentFile struct {
	FileName, ContentType string
	Data                  []byte
}

// DocumentFile returns a document's file.
func (s *Service) DocumentFile(ctx context.Context, p authz.Principal, companyID, id string) (DocumentFile, error) {
	var doc store.CompanyDocument
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		doc, err = store.New(tx).GetCompanyDocument(ctx, store.GetCompanyDocumentParams{ID: id, CompanyID: companyID})
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		return err
	})
	if err != nil {
		return DocumentFile{}, err
	}
	f, err := s.files.Open(doc.StorageKey)
	if err != nil {
		return DocumentFile{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	return DocumentFile{FileName: doc.FileName, ContentType: doc.ContentType, Data: data}, err
}

func (s *Service) mustExist(ctx context.Context, q *store.Queries, companyID string) error {
	exists, err := q.CompanyExists(ctx, companyID)
	if err != nil {
		return err
	}
	if !exists {
		return apperr.NotFound("company")
	}
	return nil
}

func randomKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
