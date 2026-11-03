package cvformats

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the formatted CV use cases.
type Service struct {
	pool  *pgxpool.Pool
	deps  Deps
	files *blob.Store
}

// TemplateView is a template as listed.
type TemplateView struct {
	ID, Name       string
	BuiltIn        bool
	UploadedByName *string
	CreatedAt      *time.Time
}

// Templates returns the built-in template first, then the company's own.
func (s *Service) Templates(ctx context.Context, p authz.Principal) ([]TemplateView, error) {
	out := []TemplateView{{ID: StandardID, Name: "HiringPlug standard", BuiltIn: true}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListTemplates(ctx)
		if err != nil {
			return err
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.UploadedBy
		}
		names, err := s.deps.Users.UserNames(ctx, tx, ids)
		if err != nil {
			return err
		}
		for _, r := range rows {
			v := TemplateView{ID: r.ID, Name: r.Name, CreatedAt: &r.CreatedAt}
			if n, ok := names[r.UploadedBy]; ok {
				v.UploadedByName = &n
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func invalidFile(message string) error {
	return apperr.Validation(apperr.FieldErrors{{Field: "file", Message: message}})
}

// Upload adds a company template, refusing one that is not a .docx or uses
// unknown placeholders.
func (s *Service) Upload(ctx context.Context, p authz.Principal, name string, data []byte) (TemplateView, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return TemplateView{}, apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "is required"}})
	}
	if !strings.HasPrefix(string(data), "PK") {
		return TemplateView{}, apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Upload a Word (.docx) file.")
	}
	found, err := Inspect(data)
	if err != nil {
		return TemplateView{}, invalidFile(err.Error())
	}
	if problem := CheckTemplate(found); problem != "" {
		return TemplateView{}, invalidFile(problem)
	}
	var out TemplateView
	var key string
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		id, err := q.NewTemplateID(ctx)
		if err != nil {
			return err
		}
		key = p.TenantID + "/cv_templates/" + id
		if err := s.files.Put(key, data); err != nil {
			return err
		}
		row, err := q.InsertTemplate(ctx, store.InsertTemplateParams{ID: id, Name: name, StorageKey: key})
		if err != nil {
			return err
		}
		out = TemplateView{ID: row.ID, Name: row.Name, UploadedByName: &p.Name, CreatedAt: &row.CreatedAt}
		return audit.Record(ctx, tx, audit.Entry{Action: "cv_template.uploaded", EntityType: "cv_template", EntityID: id, Changes: map[string]any{"name": name}})
	})
	if err != nil && key != "" {
		_ = s.files.Delete(key)
	}
	return out, err
}

// Delete removes a company template.
func (s *Service) Delete(ctx context.Context, p authz.Principal, id string) error {
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		key, err = store.New(tx).DeleteTemplate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("CV template")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "cv_template.deleted", EntityType: "cv_template", EntityID: id})
	})
	if err == nil {
		_ = s.files.Delete(key)
	}
	return err
}

// templateFile returns a template's bytes and a file name for it.
func (s *Service) templateFile(ctx context.Context, tx pgx.Tx, id string) ([]byte, string, error) {
	if id == StandardID {
		return Standard(), "HiringPlug standard CV template.docx", nil
	}
	row, err := store.New(tx).GetTemplate(ctx, id)
	if database.IsNoRows(err) {
		return nil, "", apperr.NotFound("CV template")
	}
	if err != nil {
		return nil, "", err
	}
	f, err := s.files.Open(row.StorageKey)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	return data, row.Name + ".docx", err
}

// TemplateFile returns a template to download.
func (s *Service) TemplateFile(ctx context.Context, p authz.Principal, id string) ([]byte, string, error) {
	var data []byte
	var name string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		data, name, err = s.templateFile(ctx, tx, id)
		return err
	})
	return data, name, err
}

// Options are how a formatted CV is made.
type Options struct {
	TemplateID                 string
	ApplicationID              *string
	ShowContact, IncludeSalary bool
	Summary                    string
}

// Format makes a formatted CV for a candidate and saves it with their
// documents. It returns the document's id and file name.
func (s *Service) Format(ctx context.Context, p authz.Principal, candidateID string, o Options) (string, string, error) {
	if o.IncludeSalary && !p.Can(api.PermissionCandidatesCompensationRead) {
		return "", "", apperr.New(api.ErrorCodeFORBIDDEN, "You cannot include salary: you may not see it.")
	}
	if o.TemplateID == "" {
		o.TemplateID = StandardID
	}
	var docID, fileName string
	var undo func()
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		tpl, _, err := s.templateFile(ctx, tx, o.TemplateID)
		if err != nil {
			return err
		}
		cand, err := s.deps.Candidates.ForCV(ctx, tx, p, candidateID)
		if err != nil {
			return err
		}
		agency, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		x := Context{
			ShowContact: o.ShowContact, IncludeSalary: o.IncludeSalary, Summary: o.Summary, AgencyName: agency,
			RecruiterName: p.Name, RecruiterEmail: p.Email, Today: time.Now(),
		}
		if o.ApplicationID != nil {
			apps, err := s.deps.Applications.Briefs(ctx, tx, []string{*o.ApplicationID})
			if err != nil {
				return err
			}
			app, ok := apps[*o.ApplicationID]
			if !ok || app.CandidateID != candidateID {
				return apperr.Validation(apperr.FieldErrors{{Field: "applicationId", Message: "is not one of this candidate's jobs"}})
			}
			x.JobTitle, x.CompanyName = app.JobTitle, app.CompanyName
		}
		out, err := Render(tpl, Values(cand, x))
		if err != nil {
			return invalidFile("the template could not be filled: " + err.Error())
		}
		fileName = FileName(cand, agency)
		docID, undo, err = s.deps.Documents.AddFormatted(ctx, tx, p.TenantID, candidateID, fileName, out)
		if err != nil {
			return err
		}
		// What was chosen, never the values (salary stays out of the log).
		return audit.Record(ctx, tx, audit.Entry{Action: "cv.formatted", EntityType: "candidate", EntityID: candidateID, Changes: map[string]any{
			"documentId": docID, "templateId": o.TemplateID, "showContact": o.ShowContact, "includeSalary": o.IncludeSalary,
		}})
	})
	if err != nil && undo != nil {
		undo()
	}
	return docID, fileName, err
}
