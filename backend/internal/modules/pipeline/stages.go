package pipeline

// Custom stages: the company's stage templates, and each job's own stages.
// A job follows the default template (or the standard stages) until it is
// given its own: a template's copy, or stages changed for it. Editing or
// deleting a template never changes a job that chose it (it keeps its copy).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Template is a saved stage template.
type Template struct {
	ID, Name             string
	Version, JobCount    int
	IsDefault            bool
	Steps                []Step
	CreatedAt, UpdatedAt time.Time
}

// TemplateInput is what is saved for a template.
type TemplateInput struct {
	Name      string
	IsDefault bool
	Steps     []Step
}

// Where a job's stages come from.
const (
	FromStandard = "standard" // the standard stages: no template, none default
	FromDefault  = "default"  // the default template, followed
	FromTemplate = "template" // a template's copy, chosen for the job
	FromCustom   = "custom"   // changed for the job
)

// JobStages are a job's stages and where they come from.
type JobStages struct {
	JobID, Source            string
	Version                  int // 0 while it follows the default
	TemplateID, TemplateName *string
	Steps                    []Step
}

func newStepKey() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return "s" + hex.EncodeToString(b)
}

func decodeSteps(raw []byte) []Step {
	var out []Step
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return StandardSteps()
	}
	return out
}

func checkTemplate(in TemplateInput) (TemplateInput, error) {
	in.Name = joinSpaces(in.Name)
	steps, errs := NormalizeSteps(in.Steps, newStepKey)
	switch n := len([]rune(in.Name)); {
	case n == 0:
		errs.Add("name", "enter a name, e.g. Tech hiring")
	case n > 80:
		errs.Add("name", "at most 80 characters")
	}
	in.Steps = steps
	return in, errs.Err()
}

func nameTaken(err error) error {
	if database.IsUniqueViolation(err, "stage_template_name_uq") {
		var errs apperr.FieldErrors
		errs.Add("name", "another template has this name")
		return errs.Err()
	}
	return err
}

func templateOf(r store.StageTemplate, jobs int) Template {
	return Template{ID: r.ID, Name: r.Name, Version: int(r.Version), JobCount: jobs, IsDefault: r.IsDefault, Steps: decodeSteps(r.Steps), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// Templates returns the company's stage templates, the default first.
func (s *Service) Templates(ctx context.Context, p authz.Principal) ([]Template, error) {
	var out []Template
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListStageTemplates(ctx)
		for _, r := range rows {
			out = append(out, templateOf(store.StageTemplate{ID: r.ID, Name: r.Name, Version: r.Version, IsDefault: r.IsDefault, Steps: r.Steps, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, int(r.JobCount)))
		}
		return err
	})
	return out, err
}

// CreateTemplate saves a new stage template.
func (s *Service) CreateTemplate(ctx context.Context, p authz.Principal, in TemplateInput) (Template, error) {
	in, err := checkTemplate(in)
	if err != nil {
		return Template{}, err
	}
	var out Template
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if in.IsDefault {
			if err := q.ClearDefaultStageTemplate(ctx, ""); err != nil {
				return err
			}
		}
		steps, _ := json.Marshal(in.Steps)
		row, err := q.InsertStageTemplate(ctx, store.InsertStageTemplateParams{Name: in.Name, IsDefault: in.IsDefault, Steps: steps})
		if err != nil {
			return nameTaken(err)
		}
		out = templateOf(row, 0)
		return audit.Record(ctx, tx, audit.Entry{Action: "stage_template.created", EntityType: "stage_template", EntityID: row.ID,
			Changes: map[string]any{"name": in.Name, "default": in.IsDefault, "stages": len(in.Steps)}})
	})
	return out, err
}

// UpdateTemplate changes a template (version: the one loaded). Jobs that
// chose it keep their copy; jobs following the default see the change.
func (s *Service) UpdateTemplate(ctx context.Context, p authz.Principal, id string, version int, in TemplateInput) (Template, error) {
	in, err := checkTemplate(in)
	if err != nil {
		return Template{}, err
	}
	var out Template
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetStageTemplate(ctx, id); database.IsNoRows(err) {
			return apperr.NotFound("stage template")
		} else if err != nil {
			return err
		}
		if in.IsDefault {
			if err := q.ClearDefaultStageTemplate(ctx, id); err != nil {
				return err
			}
		}
		steps, _ := json.Marshal(in.Steps)
		row, err := q.UpdateStageTemplate(ctx, store.UpdateStageTemplateParams{ID: id, Name: in.Name, IsDefault: in.IsDefault, Steps: steps,
			ExpectedVersion: int32(version)}) //nolint:gosec // a version is small
		if database.IsNoRows(err) {
			return apperr.StaleVersion("stage template")
		}
		if err != nil {
			return nameTaken(err)
		}
		out = templateOf(row, 0)
		return audit.Record(ctx, tx, audit.Entry{Action: "stage_template.updated", EntityType: "stage_template", EntityID: id,
			Changes: map[string]any{"name": in.Name, "default": in.IsDefault, "stages": len(in.Steps)}})
	})
	return out, err
}

// DeleteTemplate deletes a template; the jobs that chose it keep their stages.
func (s *Service) DeleteTemplate(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.ForgetStageTemplate(ctx, &id); err != nil {
			return err
		}
		n, err := q.DeleteStageTemplate(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("stage template")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "stage_template.deleted", EntityType: "stage_template", EntityID: id})
	})
}

// jobStages reads a job's stages in tx.
func (s *Service) jobStages(ctx context.Context, tx pgx.Tx, jobID string) (JobStages, error) {
	q := store.New(tx)
	out := JobStages{JobID: jobID}
	plan, err := q.GetJobStagePlan(ctx, jobID)
	switch {
	case err == nil:
		out.Version, out.Steps, out.TemplateID, out.Source = int(plan.Version), decodeSteps(plan.Steps), plan.TemplateID, FromCustom
		if plan.TemplateID != nil && !plan.Customized {
			out.Source = FromTemplate
		}
		if plan.TemplateID != nil {
			if t, err := q.GetStageTemplate(ctx, *plan.TemplateID); err == nil {
				out.TemplateName = &t.Name
			} else if !database.IsNoRows(err) {
				return out, err
			}
		}
		return out, nil
	case !database.IsNoRows(err):
		return out, err
	}
	def, err := q.DefaultStageTemplate(ctx)
	if database.IsNoRows(err) {
		out.Source, out.Steps = FromStandard, StandardSteps()
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Source, out.Steps, out.TemplateID, out.TemplateName = FromDefault, decodeSteps(def.Steps), &def.ID, &def.Name
	return out, nil
}

// JobStages returns a job's stages.
func (s *Service) JobStages(ctx context.Context, p authz.Principal, jobID string) (JobStages, error) {
	var out JobStages
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.requireJob(ctx, tx, jobID); err != nil {
			return err
		}
		var err error
		out, err = s.jobStages(ctx, tx, jobID)
		return err
	})
	return out, err
}

// SetJobStages gives a job its stages: a template's (templateID, no steps),
// its own (steps, from templateID or none), or none to follow the default
// again. version is the one loaded (0 while following the default).
func (s *Service) SetJobStages(ctx context.Context, p authz.Principal, jobID string, version int, templateID *string, steps []Step) (JobStages, error) {
	var out JobStages
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.requireJob(ctx, tx, jobID); err != nil {
			return err
		}
		q := store.New(tx)
		if templateID == nil && steps == nil { // follow the default again
			if version > 0 {
				n, err := q.DeleteJobStagePlan(ctx, store.DeleteJobStagePlanParams{JobOrderID: jobID, ExpectedVersion: int32(version)}) //nolint:gosec // small
				if err != nil {
					return err
				}
				if n == 0 {
					return apperr.StaleVersion("job stages")
				}
			}
		} else {
			var from []Step
			if templateID != nil {
				t, err := q.GetStageTemplate(ctx, *templateID)
				if database.IsNoRows(err) {
					var errs apperr.FieldErrors
					errs.Add("templateId", "no such template")
					return errs.Err()
				}
				if err != nil {
					return err
				}
				from = decodeSteps(t.Steps)
			}
			chosen := from
			if steps != nil {
				var errs apperr.FieldErrors
				if chosen, errs = NormalizeSteps(steps, newStepKey); len(errs) > 0 {
					return errs.Err()
				}
			}
			raw, _ := json.Marshal(chosen)
			_, err := q.UpsertJobStagePlan(ctx, store.UpsertJobStagePlanParams{JobOrderID: jobID, TemplateID: templateID, Steps: raw,
				Customized: templateID == nil || !SameSteps(chosen, from), ExpectedVersion: int32(version)}) //nolint:gosec // small
			if database.IsNoRows(err) {
				return apperr.StaleVersion("job stages")
			}
			if err != nil {
				return err
			}
		}
		var err error
		if out, err = s.jobStages(ctx, tx, jobID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "job.stages_changed", EntityType: "job", EntityID: jobID,
			Changes: map[string]any{"source": out.Source, "templateId": out.TemplateID, "stages": len(out.Steps)}})
	})
	return out, err
}

func joinSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }
