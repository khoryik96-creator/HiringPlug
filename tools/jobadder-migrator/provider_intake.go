package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type SourceImport struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Records int64  `json:"records"`
	Message string `json:"message"`
	Blob    Blob   `json:"blob"`
}

func (s *Store) addImport(b Blob, name, sourceType string) (int64, error) {
	raw, _ := json.Marshal(map[string]string{"name": name})
	encrypted, err := seal(s.key, raw)
	if err != nil {
		return 0, err
	}
	r, err := s.db.Exec(`INSERT INTO source_imports(blob_id,source_type,metadata,status,created) VALUES(?,?,?,'processing',?)`, b.ID, sourceType, encrypted, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}
func (s *Store) sourceImports() ([]SourceImport, error) {
	rows, err := s.db.Query(`SELECT i.id,i.metadata,i.status,i.records,i.message,b.id,b.sha,b.size FROM source_imports i JOIN blobs b ON b.id=i.blob_id ORDER BY i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var imports []SourceImport
	for rows.Next() {
		var v SourceImport
		var sealed []byte
		if err := rows.Scan(&v.ID, &sealed, &v.Status, &v.Records, &v.Message, &v.Blob.ID, &v.Blob.SHA, &v.Blob.Size); err != nil {
			return nil, err
		}
		raw, err := unseal(s.key, sealed)
		if err != nil {
			return nil, err
		}
		var metadata map[string]string
		if err = json.Unmarshal(raw, &metadata); err != nil {
			return nil, err
		}
		v.Name = metadata["name"]
		imports = append(imports, v)
	}
	return imports, rows.Err()
}
func (s *Store) addMember(id int64, b Blob, name, kind string) (int64, error) {
	raw, _ := json.Marshal(map[string]string{"name": name})
	encrypted, err := seal(s.key, raw)
	if err != nil {
		return 0, err
	}
	r, err := s.db.Exec(`INSERT INTO source_members(import_id,blob_id,metadata,kind,status) VALUES(?,?,?,?,'processing')`, id, b.ID, encrypted, kind)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

// The full original is already durable, including unsupported backup formats.
// Indexing is separate and never implies a provider backup is complete.
func (s *Store) ingestSource(ctx context.Context, id int64, b Blob, name, entity, idField string) error {
	var count int64
	var review bool
	err := s.ingestMember(ctx, id, b, name, entity, idField, 0, &count, &review)
	status, message := "indexed_unverified", "Original retained. Source identity, relationships and coverage require reconciliation."
	if review {
		status = "needs_adapter"
		message = "Original retained. One or more files need a format adapter or logical provider export."
	}
	if err != nil {
		status = "needs_review"
		message = "Original retained; indexing interrupted or format could not be validated. Retry intake or supply a supported logical export."
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM source_rows WHERE member_id IN (SELECT id FROM source_members WHERE import_id=?)`, id).Scan(&count); err != nil {
		return err
	}
	_, saveErr := s.db.Exec(`UPDATE source_imports SET status=?,records=?,message=? WHERE id=?`, status, count, message, id)
	if saveErr != nil {
		return saveErr
	}
	return err
}

func (s *Store) ingestMember(ctx context.Context, importID int64, b Blob, name, entity, idField string, depth int, count *int64, review *bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	kind := strings.ToLower(path.Ext(name))
	member, err := s.addMember(importID, b, name, kind)
	if err != nil {
		return err
	}
	status := "preserved_needs_adapter"
	rows := int64(0)
	defer func() {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM source_rows WHERE member_id=?`, member).Scan(&rows)
		_, _ = s.db.Exec(`UPDATE source_members SET status=?,records=? WHERE id=?`, status, rows, member)
	}()
	if kind == ".zip" {
		if depth >= 3 {
			return errors.New("nested archive depth exceeded")
		}
		f, err := createArchiveFile()
		if err != nil {
			return err
		}
		defer f.Close()
		r, err := s.OpenBlob(b.ID)
		if err != nil {
			return err
		}
		defer r.Close()
		if err = copyContext(ctx, f, r); err != nil {
			return err
		}
		z, err := zip.NewReader(f, b.Size)
		if err != nil {
			return err
		}
		if len(z.File) > 100000 {
			return errors.New("archive entry safety bound exceeded")
		}
		var expanded uint64
		for _, entry := range z.File {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.FileInfo().IsDir() {
				continue
			}
			if !safeMemberName(entry.Name) || entry.Mode()&os.ModeSymlink != 0 {
				return errors.New("unsafe archive member")
			}
			expanded += entry.UncompressedSize64
			if expanded > uint64(maxSourceBytes) || entry.UncompressedSize64 > uint64(maxSourceBytes) || (entry.UncompressedSize64 > 100<<20 && entry.UncompressedSize64/(entry.CompressedSize64+1) > 1000) {
				return errors.New("archive expansion safety bound exceeded")
			}
			r, err := entry.Open()
			if err != nil {
				return err
			}
			object, saveErr := s.PutBlob(ctx, r)
			closeErr := r.Close()
			if saveErr != nil {
				return saveErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err = s.ingestMember(ctx, importID, object, entry.Name, entity, idField, depth+1, count, review); err != nil {
				return err
			}
		}
		status = "members_preserved"
		return nil
	}
	switch kind {
	case ".csv", ".json", ".jsonl", ".ndjson":
		r, err := s.OpenBlob(b.ID)
		if err != nil {
			return err
		}
		defer r.Close()
		rows, err = s.indexSourceRows(ctx, member, r, kind, entity, idField)
		*count += rows
		if err != nil {
			status = "index_incomplete"
			return err
		}
		status = "indexed_unverified"
	default:
		*review = true
	}
	return nil
}

func safeMemberName(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return false
		}
	}
	return path.Clean(name) != "."
}

// Verify the native object all the way through its authenticated final frame.
func (s *Store) verifyBlob(ctx context.Context, id string) error {
	r, err := s.OpenBlob(id)
	if err != nil {
		return err
	}
	defer r.Close()
	return copyContext(ctx, io.Discard, r)
}
