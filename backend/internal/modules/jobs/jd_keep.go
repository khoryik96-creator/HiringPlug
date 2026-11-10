package jobs

// Uploaded job descriptions are kept with their job: listed on the job page
// to view or download as uploaded, and deleted by someone who may edit the
// job. One read on the Add job form joins the job made from it.

import (
	"bytes"
	"context"
	"io"
	"mime"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// JDFiles returns the job descriptions kept with a job, newest first.
func (s *Service) JDFiles(ctx context.Context, p authz.Principal, jobID string) ([]JDFile, error) {
	var out []JDFile
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		exists, err := s.Exists(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if !exists {
			return apperr.NotFound("job")
		}
		rows, err := store.New(tx).ListJobJdFiles(ctx, &jobID)
		if err != nil {
			return err
		}
		out = make([]JDFile, 0, len(rows))
		for _, r := range rows {
			f, err := jdFrom(store.GetJdFileRow(r))
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return nil
	})
	return out, err
}

// DeleteJD deletes an uploaded job description and its file. What was filled
// in from it (the job, an ad) stays as it is.
func (s *Service) DeleteJD(ctx context.Context, p authz.Principal, id string) error {
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).DeleteJdFile(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("job description")
		}
		if err != nil {
			return err
		}
		key = row.StorageKey
		entity, entityID := "jd_file", id
		if row.JobID != nil {
			entity, entityID = "job", *row.JobID
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job.jd_deleted", EntityType: entity, EntityID: entityID,
			Changes: map[string]any{"jdFileId": id, "fileName": row.FileName},
		})
	})
	if err == nil {
		if err := s.files.Delete(key); err != nil {
			logx.From(ctx).Warn("could not remove a deleted job description's file", "error", err)
		}
	}
	return err
}

// JDDownload is an uploaded job description's file.
type JDDownload struct {
	FileName, ContentType string
	Data                  []byte
}

// JDFileData returns an uploaded job description's file, as uploaded.
func (s *Service) JDFileData(ctx context.Context, p authz.Principal, id string) (JDDownload, error) {
	var row store.GetJdFileStorageRow
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).GetJdFileStorage(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("job description")
		}
		return err
	})
	if err != nil {
		return JDDownload{}, err
	}
	f, err := s.files.Open(row.StorageKey)
	if err != nil {
		return JDDownload{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	return JDDownload{FileName: row.FileName, ContentType: row.ContentType, Data: data}, err
}

// linkJD keeps the job description the Add job form was filled in from with
// the new job. It must be one read without a job.
func linkJD(ctx context.Context, tx pgx.Tx, jdFileID, jobID string) error {
	n, err := store.New(tx).LinkJdFileToJob(ctx, store.LinkJdFileToJobParams{ID: jdFileID, JobID: &jobID})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.Validation(apperr.FieldErrors{{Field: "jdFileId", Message: "is not a job description waiting for its job"}})
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "job.jd_kept", EntityType: "job", EntityID: jobID, Changes: map[string]any{"jdFileId": jdFileID},
	})
}

// ListJobDescriptions handles GET /v1/jobs/{jobId}/jd-files.
func (h *HTTP) ListJobDescriptions(ctx context.Context, req api.ListJobDescriptionsRequestObject) (api.ListJobDescriptionsResponseObject, error) {
	files, err := h.svc.JDFiles(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	out := api.ListJobDescriptions200JSONResponse{Items: make([]api.JdFile, len(files))}
	for i, f := range files {
		out.Items[i] = jdToAPI(f)
	}
	return out, nil
}

// DeleteJobDescription handles DELETE /v1/jd-files/{jdFileId}.
func (h *HTTP) DeleteJobDescription(ctx context.Context, req api.DeleteJobDescriptionRequestObject) (api.DeleteJobDescriptionResponseObject, error) {
	if err := h.svc.DeleteJD(ctx, authz.MustFromContext(ctx), req.JdFileId); err != nil {
		return nil, err
	}
	return api.DeleteJobDescription204Response{}, nil
}

// DownloadJobDescription handles GET /v1/jd-files/{jdFileId}/file.
func (h *HTTP) DownloadJobDescription(ctx context.Context, req api.DownloadJobDescriptionRequestObject) (api.DownloadJobDescriptionResponseObject, error) {
	f, err := h.svc.JDFileData(ctx, authz.MustFromContext(ctx), req.JdFileId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})
	return api.DownloadJobDescription200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(f.Data), ContentLength: int64(len(f.Data)),
		Headers: api.DownloadJobDescription200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
