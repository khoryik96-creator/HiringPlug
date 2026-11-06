package contacts

// Checking where a contact works now: their LinkedIn profile saved as PDF is
// stored, read by the AI worker (contact.read_linkedin, the rules only) and
// sent back on contact.linkedin_read; the jobs it lists are kept, compared
// with the contact's company, and the PDF is deleted.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/linkedin"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

const (
	profileLinkTTL = 30 * time.Minute
	// readingGivenUp is when a check still being read counts as failed: its
	// job died (worker down, link expired), and the user can drop the PDF again.
	readingGivenUp = time.Hour
)

// EmploymentCheck is a check and, once read, what it found.
type EmploymentCheck struct {
	ID, Status, FileName string
	Error                *string
	Profile              *Profile
	Employment           Employment
	CheckedAt            time.Time
}

func checkFrom(row store.ContactEmploymentCheck, ct Contact) (*EmploymentCheck, error) {
	out := &EmploymentCheck{
		ID: row.ID, Status: row.Status, FileName: row.FileName, Error: row.Error,
		Employment: Employment{Verdict: "unknown"}, CheckedAt: row.CreatedAt,
	}
	if row.Status == "reading" && time.Since(row.CreatedAt) > readingGivenUp {
		msg := "It was not read in time: drop the PDF again."
		out.Status, out.Error = "failed", &msg
	}
	if len(row.Result) > 0 {
		var p Profile
		if err := json.Unmarshal(row.Result, &p); err != nil {
			return nil, err
		}
		out.Profile = &p
		// Compared when shown: the contact's company or name may have changed since.
		out.Employment = Compare(p, ct.CompanyName, ct.Details.FirstName, ct.Details.LastName, ct.Details.LinkedinURL)
	}
	return out, nil
}

// CheckEmployment stores a contact's LinkedIn profile (a PDF) and queues
// reading it.
func (s *Service) CheckEmployment(ctx context.Context, p authz.Principal, contactID, fileName string, data []byte) (EmploymentCheck, error) {
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return EmploymentCheck{}, apperr.Validation(apperr.FieldErrors{{Field: "file",
			Message: "must be the PDF LinkedIn makes (on their profile: More > Save to PDF)"}})
	}
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if name == "" || name == "." {
		name = "Profile.pdf"
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	var out EmploymentCheck
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, contactID)
		if err != nil {
			return err
		}
		id, err := q.NewEmploymentCheckID(ctx)
		if err != nil {
			return err
		}
		key = p.TenantID + "/" + id
		if err := s.files.Put(key, data); err != nil {
			key = ""
			return err
		}
		check, err := q.InsertEmploymentCheck(ctx, store.InsertEmploymentCheckParams{ID: id, ContactID: row.ID, FileName: name, StorageKey: &key})
		if err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindContactReadLinkedin, api.ContactReadLinkedinJob{
			CheckId: id, DownloadUrl: s.files.SignedURL(key, profileLinkTTL), FileName: name, SizeBytes: int64(len(data)),
		}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "contact.employment_check_started", EntityType: "contact", EntityID: row.ID, Changes: map[string]any{"fileName": name},
		}); err != nil {
			return err
		}
		ct, err := s.withCompanyName(ctx, tx, fromRow(row))
		if err != nil {
			return err
		}
		e, err := checkFrom(check, ct)
		if err == nil {
			out = *e
		}
		return err
	})
	if err != nil && key != "" {
		err = errors.Join(err, s.files.Delete(key)) // nothing refers to it
	}
	return out, err
}

// saveLinkedinRead handles contact.linkedin_read: what the profile says.
func (s *Service) saveLinkedinRead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.ContactLinkedinReadJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	check, err := q.GetEmploymentCheck(ctx, r.CheckId)
	if database.IsNoRows(err) || (err == nil && check.Status != "reading") {
		return nil // deleted with its contact, or a repeated result
	}
	if err != nil {
		return err
	}
	params := store.SaveEmploymentCheckParams{ID: check.ID, Status: "failed"}
	var p Profile
	if profile, err := r.Profile.Get(); err == nil {
		p = profileFrom(profile)
		if len(p.Positions) == 0 && p.Name == nil {
			msg := "No name or jobs were found in it: is it a LinkedIn profile saved as PDF?"
			params.Error = &msg
		} else if params.Result, err = json.Marshal(p); err != nil {
			return err
		} else {
			params.Status = "read"
		}
	} else {
		msg := "It could not be read."
		if m, err := r.ErrorMessage.Get(); err == nil && m != "" {
			msg = clip("It could not be read: "+m, 500)
		}
		params.Error = &msg
	}
	if _, err := q.SaveEmploymentCheck(ctx, params); err != nil {
		return err
	}
	if check.StorageKey != nil {
		if err := s.files.Delete(*check.StorageKey); err != nil {
			logx.From(ctx).Warn("linkedin profile pdf not deleted", "check_id", check.ID, "error", err)
		}
	}
	changes := map[string]any{"status": params.Status}
	if params.Status == "read" && p.URL != nil {
		// The profile gives the contact's LinkedIn address when it was missing.
		if n, err := q.FillLinkedinURL(ctx, store.FillLinkedinURLParams{ID: check.ContactID, LinkedinUrl: p.URL}); err != nil {
			return err
		} else if n > 0 {
			changes["linkedinUrl"] = map[string]any{"from": nil, "to": *p.URL}
		}
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "contact.employment_checked", EntityType: "contact", EntityID: check.ContactID, Changes: changes})
}

// profileFrom keeps the name, the LinkedIn address and the jobs of what the
// rules read (values only: quotes are not needed here).
func profileFrom(pp api.ParsedProfile) Profile {
	out := Profile{Positions: []Position{}}
	if v, err := pp.FullName.Get(); err == nil {
		out.Name = &v.Value
	}
	if v, err := pp.LinkedinUrl.Get(); err == nil {
		if u, ok := linkedin.Normalize(v.Value); ok {
			out.URL = &u
		}
	}
	for _, j := range pp.Experience {
		if len(out.Positions) == maxPositions {
			break
		}
		pos := Position{Current: j.Current}
		if v, err := j.Title.Get(); err == nil {
			pos.Title = &v.Value
		}
		if v, err := j.Company.Get(); err == nil {
			pos.Company = &v.Value
		}
		if v, err := j.StartDate.Get(); err == nil {
			pos.StartDate = &v
		}
		if v, err := j.EndDate.Get(); err == nil {
			pos.EndDate = &v
		}
		if pos.Title != nil || pos.Company != nil {
			out.Positions = append(out.Positions, pos)
		}
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// CheckContactEmployment handles POST /v1/contacts/{contactId}/linkedin-profile.
func (h *HTTP) CheckContactEmployment(ctx context.Context, req api.CheckContactEmploymentRequestObject) (api.CheckContactEmploymentResponseObject, error) {
	name, data, err := readFile(req.Body, h.maxUploadBytes)
	if err != nil {
		return nil, err
	}
	e, err := h.svc.CheckEmployment(ctx, authz.MustFromContext(ctx), req.ContactId, name, data)
	if err != nil {
		return nil, err
	}
	return api.CheckContactEmployment202JSONResponse(employmentToAPI(e)), nil
}

func readFile(r *multipart.Reader, limit int64) (string, []byte, error) {
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

func employmentToAPI(e EmploymentCheck) api.ContactEmployment {
	out := api.ContactEmployment{
		Id: e.ID, Status: api.ContactEmploymentStatus(e.Status), Verdict: api.ContactEmploymentVerdict(e.Employment.Verdict),
		FileName: e.FileName, Error: c.OrNull(e.Error), CheckedAt: e.CheckedAt, Positions: []api.ContactPosition{},
		NameMatches: c.OrNull(e.Employment.NameMatches), UrlMatches: c.OrNull(e.Employment.URLMatches),
	}
	out.ProfileName.SetNull()
	out.ProfileUrl.SetNull()
	out.Current.SetNull()
	if e.Profile != nil {
		out.ProfileName, out.ProfileUrl = c.OrNull(e.Profile.Name), c.OrNull(e.Profile.URL)
		for _, pos := range e.Profile.Positions {
			out.Positions = append(out.Positions, positionToAPI(pos))
		}
	}
	if cur := e.Employment.Current; cur != nil {
		out.Current.Set(positionToAPI(*cur))
	}
	return out
}

func positionToAPI(p Position) api.ContactPosition {
	return api.ContactPosition{
		Title: c.OrNull(p.Title), Company: c.OrNull(p.Company), StartDate: c.OrNull(p.StartDate), EndDate: c.OrNull(p.EndDate), Current: p.Current,
	}
}
