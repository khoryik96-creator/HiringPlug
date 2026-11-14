package jobs

// Job descriptions uploaded to fill in an ad: a text file is read at once;
// a PDF or Word file goes to the AI worker for its text (job.read_jd), and
// comes back on job.jd_read. Either way the ad's details are then read from
// the text (ReadJD) and kept with the file for the ad form to use.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

const jdLinkTTL = 7 * 24 * time.Hour // as long as a CV's: the AI worker may be busy or down for a while

// JDFile is an uploaded job description and, once read, the ad details in it.
type JDFile struct {
	ID, FileName, Status string
	ContentType          string
	SizeBytes            int
	JobID                *string // nil: read before its job was added
	Error                *string
	Details              *JDDetails
	CreatedAt            time.Time
}

// jdKind is what an uploaded file is, from its bytes (and, for a plain text
// file, its name): the same files as a CV (filekind).
func jdKind(name string, data []byte) (contentType string, ok bool) {
	return filekind.Detect(name, data)
}

// UploadJD stores a job description for a job (or, with jobID "", for a job
// not added yet: the Add job form) and starts reading it.
func (s *Service) UploadJD(ctx context.Context, p authz.Principal, jobID, fileName string, data []byte) (JDFile, error) {
	ctype, ok := jdKind(fileName, data)
	if !ok {
		return JDFile{}, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "must be " + filekind.Accepted}})
	}
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if name == "" || name == "." {
		name = "Job description"
	}
	name = clipRunes(name, 255)
	var out JDFile
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var job *string
		if jobID != "" {
			exists, err := s.Exists(ctx, tx, jobID)
			if err != nil {
				return err
			}
			if !exists {
				return apperr.NotFound("job")
			}
			job = &jobID
		}
		q := store.New(tx)
		id, err := q.NewJdFileID(ctx)
		if err != nil {
			return err
		}
		key = p.TenantID + "/" + id
		if err := s.files.Put(key, data); err != nil {
			key = ""
			return err
		}
		params := store.InsertJdFileParams{
			ID: id, JobID: job, FileName: name, ContentType: ctype, SizeBytes: int32(len(data)), //nolint:gosec // capped by the upload limit
			StorageKey: key, Status: string(api.JdFileStatusReading),
		}
		if filekind.ReadHere(ctype) { // a text, RTF or ODT file: read here, nothing for the worker to do
			text, err := filekind.Text(ctype, data)
			if err != nil {
				return apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "has no readable text"}})
			}
			details, err := json.Marshal(ReadJD(text, time.Now()))
			if err != nil {
				return err
			}
			params.Status, params.Text, params.Details = string(api.JdFileStatusRead), &text, details
		}
		row, err := q.InsertJdFile(ctx, params)
		if err != nil {
			return err
		}
		if !filekind.ReadHere(ctype) {
			if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindJobReadJd, api.JobReadJdJob{
				JdFileId: id, DownloadUrl: s.files.SignedURL(key, jdLinkTTL), ContentType: ctype, FileName: name, SizeBytes: int64(len(data)),
			}); err != nil {
				return err
			}
		}
		entity, entityID := "job", jobID
		if job == nil {
			entity, entityID = "jd_file", id
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "job.jd_uploaded", EntityType: entity, EntityID: entityID, Changes: map[string]any{"fileName": name},
		}); err != nil {
			return err
		}
		out, err = jdFrom(store.GetJdFileRow(row))
		return err
	})
	if err != nil && key != "" {
		err = errors.Join(err, s.files.Delete(key)) // nothing refers to it
	}
	return out, err
}

// JD returns an uploaded job description.
func (s *Service) JD(ctx context.Context, p authz.Principal, id string) (JDFile, error) {
	var out JDFile
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetJdFile(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("job description")
		}
		if err != nil {
			return err
		}
		out, err = jdFrom(row)
		return err
	})
	return out, err
}

// saveJDRead handles job.jd_read: the worker's text, and the details read from it.
func (s *Service) saveJDRead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.JobJdReadJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	params := store.SaveJdReadParams{ID: result.JdFileId, Status: string(api.JdFileStatusFailed)}
	text, _ := result.Text.Get()
	text = strings.TrimSpace(text)
	switch {
	case result.Status == api.ExtractionStatusExtracted && text != "":
		details, err := json.Marshal(ReadJD(text, time.Now()))
		if err != nil {
			return err
		}
		params.Status, params.Text, params.Details = string(api.JdFileStatusRead), &text, details
	case result.Status == api.ExtractionStatusNeedsOcr:
		msg := "It is a scan with no text in it: upload the Word or PDF file the client sent, or paste the text."
		params.Error = &msg
	default:
		msg := "It could not be read."
		if m, err := result.ErrorMessage.Get(); err == nil && m != "" {
			msg = "It could not be read: " + clipRunes(m, 400)
		}
		params.Error = &msg
	}
	n, err := store.New(tx).SaveJdRead(ctx, params)
	if err == nil && n == 0 {
		logx.From(ctx).Info("job description no longer exists; dropping its text", "jd_file_id", result.JdFileId)
	}
	return err
}

func jdFrom(r store.GetJdFileRow) (JDFile, error) {
	out := JDFile{
		ID: r.ID, JobID: r.JobID, FileName: r.FileName, ContentType: r.ContentType, SizeBytes: int(r.SizeBytes),
		Status: r.Status, Error: r.Error, CreatedAt: r.CreatedAt,
	}
	if len(r.Details) > 0 {
		var d JDDetails
		if err := json.Unmarshal(r.Details, &d); err != nil {
			return out, err
		}
		out.Details = &d
	}
	return out, nil
}

// UploadJobDescription handles POST /v1/jobs/{jobId}/jd-files.
func (h *HTTP) UploadJobDescription(ctx context.Context, req api.UploadJobDescriptionRequestObject) (api.UploadJobDescriptionResponseObject, error) {
	name, data, err := readJDForm(req.Body, h.maxUploadBytes)
	if err != nil {
		return nil, err
	}
	f, err := h.svc.UploadJD(ctx, authz.MustFromContext(ctx), req.JobId, name, data)
	if err != nil {
		return nil, err
	}
	return api.UploadJobDescription201JSONResponse(jdToAPI(f)), nil
}

// UploadNewJobDescription handles POST /v1/jd-files (the Add job form).
func (h *HTTP) UploadNewJobDescription(ctx context.Context, req api.UploadNewJobDescriptionRequestObject) (api.UploadNewJobDescriptionResponseObject, error) {
	name, data, err := readJDForm(req.Body, h.maxUploadBytes)
	if err != nil {
		return nil, err
	}
	f, err := h.svc.UploadJD(ctx, authz.MustFromContext(ctx), "", name, data)
	if err != nil {
		return nil, err
	}
	return api.UploadNewJobDescription201JSONResponse(jdToAPI(f)), nil
}

// GetJobDescriptionFile handles GET /v1/jd-files/{jdFileId}.
func (h *HTTP) GetJobDescriptionFile(ctx context.Context, req api.GetJobDescriptionFileRequestObject) (api.GetJobDescriptionFileResponseObject, error) {
	f, err := h.svc.JD(ctx, authz.MustFromContext(ctx), req.JdFileId)
	if err != nil {
		return nil, err
	}
	return api.GetJobDescriptionFile200JSONResponse(jdToAPI(f)), nil
}

func readJDForm(r *multipart.Reader, limit int64) (string, []byte, error) {
	var name string
	var data []byte
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data form"}})
		}
		if part.FormName() != "file" {
			return "", nil, apperr.Validation(apperr.FieldErrors{{Field: part.FormName(), Message: "unexpected field"}})
		}
		name = part.FileName()
		if data, err = io.ReadAll(io.LimitReader(part, limit+1)); err != nil {
			return "", nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "could not be read"}})
		}
		if int64(len(data)) > limit {
			return "", nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, "The file is too large.")
		}
	}
	if len(data) == 0 {
		return "", nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "is required and must not be empty"}})
	}
	return name, data, nil
}

func jdToAPI(f JDFile) api.JdFile {
	out := api.JdFile{Id: f.ID, JobId: c.OrNull(f.JobID), FileName: f.FileName, ContentType: f.ContentType, SizeBytes: f.SizeBytes, Status: api.JdFileStatus(f.Status), CreatedAt: f.CreatedAt, Error: c.OrNull(f.Error)}
	out.Details.SetNull()
	if d := f.Details; d != nil {
		v := api.JdDetails{
			Description: d.Description, Title: c.OrNull(nonEmpty(d.Title)), Location: c.OrNull(nonEmpty(d.Location)),
			SalaryText: c.OrNull(nonEmpty(d.SalaryText)), Country: c.OrNull(nonEmpty(d.Country)),
			WorkType:        c.OrNull(c.Convert[api.WorkType](nonEmpty(d.WorkType))),
			WorkArrangement: c.OrNull(c.Convert[api.WorkArrangement](nonEmpty(d.WorkArrangement))),
		}
		v.ClosesOn.SetNull()
		if d.ClosesOn != nil {
			v.ClosesOn.Set(openapi_types.Date{Time: *d.ClosesOn})
		}
		out.Details.Set(v)
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
