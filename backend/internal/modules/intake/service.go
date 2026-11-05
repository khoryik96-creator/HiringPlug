package intake

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/intake/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the intake use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
}

// Item is one imported file as the user sees it.
type Item struct {
	ID, FileName, Status string
	CandidateID          *string
	CandidateName        *string // nil when none, or one the user may not see
	Error                *string
	UpdatedAt            time.Time
}

// Import is an import with its files.
type Import struct {
	ID, Source string
	CreatedAt  time.Time
	Items      []Item
}

// Create starts an import whose candidates get source.
func (s *Service) Create(ctx context.Context, p authz.Principal, source string) (Import, error) {
	var out Import
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := s.deps.Sources.HasOption(ctx, tx, "candidate_source", source)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Validation(apperr.FieldErrors{{Field: "source", Message: "is not one of your company's sources (Settings)"}})
		}
		row, err := store.New(tx).InsertImport(ctx, source)
		if err != nil {
			return err
		}
		out = Import{ID: row.ID, Source: row.Source, CreatedAt: row.CreatedAt, Items: []Item{}}
		return audit.Record(ctx, tx, audit.Entry{Action: "cv_import.started", EntityType: "cv_import", EntityID: row.ID, Changes: map[string]any{"source": source}})
	})
	return out, err
}

// AddFile adds one file to an import: a candidate named after it, owned by
// the user, with the file read like any CV. A file that is not a CV file is
// recorded as failed (the import shows it), and makes no candidate.
func (s *Service) AddFile(ctx context.Context, p authz.Principal, importID, fileName string, data []byte) (Item, error) {
	var out Item
	var undo func()
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		imp, err := q.GetImport(ctx, importID)
		if database.IsNoRows(err) {
			return apperr.NotFound("CV import")
		}
		if err != nil {
			return err
		}
		params := store.InsertItemParams{ImportID: importID, FileName: fileName, Status: Reading}
		if !s.deps.Documents.IsCVFile(data) { // recorded, so the import shows it; no candidate
			msg := "Not a PDF or Word (.docx or .doc) file."
			params.Status, params.Error = Failed, &msg
		} else {
			owner := p.UserID
			candidateID, err := s.deps.Candidates.CreatePlaceholder(ctx, tx, fileName, imp.Source, &owner)
			if err != nil {
				return err
			}
			docID, u, err := s.deps.Documents.AddFileAs(ctx, tx, p.TenantID, candidateID, documentSource(imp.Source), fileName, data)
			undo = u
			if err != nil {
				return err
			}
			params.DocumentID, params.CandidateID = &docID, &candidateID
		}
		row, err := q.InsertItem(ctx, params)
		if err != nil {
			return err
		}
		names := map[string]string{}
		if row.CandidateID != nil { // named after the file until read
			if names, err = s.deps.Candidates.Names(ctx, tx, []string{*row.CandidateID}); err != nil {
				return err
			}
		}
		out = itemFromRow(row, names)
		return audit.Record(ctx, tx, audit.Entry{
			Action: "cv_import.file_added", EntityType: "cv_import", EntityID: importID, Changes: map[string]any{"status": params.Status},
		})
	})
	if err != nil && undo != nil {
		undo()
	}
	return out, err
}

// documentSource is the document's source for a candidate source: a
// LinkedIn import's files are LinkedIn PDFs.
func documentSource(candidateSource string) string {
	if candidateSource == "linkedin" {
		return "linkedin"
	}
	return "original"
}

// readDone handles the end of reading an imported file.
func (s *Service) readDone(ctx context.Context, tx pgx.Tx, documentID string, profile *api.ParsedProfile) error {
	q := store.New(tx)
	item, err := q.ReadingItemByDocument(ctx, &documentID)
	if database.IsNoRows(err) {
		return nil // not an imported file, or already done
	}
	if err != nil {
		return err
	}
	finish := store.FinishItemParams{ID: item.ID, Status: Unread, CandidateID: item.CandidateID}
	if profile != nil && item.CandidateID != nil {
		owner, err := s.deps.Candidates.FillFromCV(ctx, tx, *item.CandidateID, *profile)
		if err != nil {
			return err
		}
		finish.Status = Created
		if owner != "" { // one candidate per email: the file joins them
			if err := s.deps.Candidates.MergeInto(ctx, tx, owner, *item.CandidateID); err != nil {
				return err
			}
			finish.Status, finish.CandidateID = Added, &owner
		}
	}
	if err := q.FinishItem(ctx, finish); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "cv_import.file_read", EntityType: "cv_import", EntityID: item.ImportID, Changes: map[string]any{"status": finish.Status},
	})
}

// List returns the user's latest imports with their files.
func (s *Service) List(ctx context.Context, p authz.Principal) ([]Import, error) {
	var out []Import
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListImports(ctx)
		if err != nil {
			return err
		}
		out, err = s.withItems(ctx, tx, rows)
		return err
	})
	return out, err
}

// Get returns one of the user's imports.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (Import, error) {
	var out Import
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetImport(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("CV import")
		}
		if err != nil {
			return err
		}
		all, err := s.withItems(ctx, tx, []store.CvImport{row})
		out = all[0]
		return err
	})
	return out, err
}

func (s *Service) withItems(ctx context.Context, tx pgx.Tx, imports []store.CvImport) ([]Import, error) {
	ids := make([]string, len(imports))
	for i, imp := range imports {
		ids[i] = imp.ID
	}
	rows, err := store.New(tx).ListItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	var candidateIDs []string
	for _, r := range rows {
		if r.CandidateID != nil {
			candidateIDs = append(candidateIDs, *r.CandidateID)
		}
	}
	names, err := s.deps.Candidates.Names(ctx, tx, candidateIDs)
	if err != nil {
		return nil, err
	}
	byImport := map[string][]Item{}
	for _, r := range rows {
		byImport[r.ImportID] = append(byImport[r.ImportID], itemFromRow(r, names))
	}
	out := make([]Import, len(imports))
	for i, imp := range imports {
		items := byImport[imp.ID]
		if items == nil {
			items = []Item{}
		}
		out[i] = Import{ID: imp.ID, Source: imp.Source, CreatedAt: imp.CreatedAt, Items: items}
	}
	return out, nil
}

func itemFromRow(r store.CvImportItem, names map[string]string) Item {
	it := Item{ID: r.ID, FileName: r.FileName, Status: r.Status, CandidateID: r.CandidateID, Error: r.Error, UpdatedAt: r.UpdatedAt}
	if r.CandidateID != nil {
		if n, ok := names[*r.CandidateID]; ok {
			it.CandidateName = &n
		} else {
			it.CandidateID = nil // one the user may not see
		}
	}
	return it
}
