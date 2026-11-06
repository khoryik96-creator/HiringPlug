package settings

// Policies: the company's choices on how the app treats sensitive
// information (who sees emails that show salary).

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const emailSalaryKey = "email_salary_visibility"

// Who sees a sent email that shows salary.
const (
	SalaryRoles = "salary_roles" // only roles that may see salaries (the default)
	Everyone    = "everyone"     // everyone who may read emails
)

// EmailSalaryVisibility is the company's choice, SalaryRoles until it makes one.
func (m *Module) EmailSalaryVisibility(ctx context.Context, tx pgx.Tx) (string, error) {
	return emailSalaryVisibility(ctx, tx)
}

func emailSalaryVisibility(ctx context.Context, tx pgx.Tx) (string, error) {
	v, err := store.New(tx).GetPolicy(ctx, emailSalaryKey)
	switch {
	case database.IsNoRows(err):
		return SalaryRoles, nil
	case err != nil:
		return "", err
	case v == Everyone:
		return Everyone, nil
	default:
		return SalaryRoles, nil
	}
}

// Policies returns the company's choices.
func (s *Service) Policies(ctx context.Context, p authz.Principal) (string, error) {
	var out string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = emailSalaryVisibility(ctx, tx)
		return err
	})
	return out, err
}

// SetPolicies changes them.
func (s *Service) SetPolicies(ctx context.Context, p authz.Principal, emailSalary string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).SetPolicy(ctx, store.SetPolicyParams{Key: emailSalaryKey, Value: emailSalary}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "settings.policies_changed", EntityType: "settings", EntityID: p.TenantID,
			Changes: map[string]any{"emailSalaryVisibility": emailSalary}})
	})
}

// GetPolicies handles GET /v1/settings/policies.
func (h *HTTP) GetPolicies(ctx context.Context, _ api.GetPoliciesRequestObject) (api.GetPoliciesResponseObject, error) {
	v, err := h.svc.Policies(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetPolicies200JSONResponse{EmailSalaryVisibility: api.EmailSalaryVisibility(v)}, nil
}

// PutPolicies handles PUT /v1/settings/policies.
func (h *HTTP) PutPolicies(ctx context.Context, req api.PutPoliciesRequestObject) (api.PutPoliciesResponseObject, error) {
	p := authz.MustFromContext(ctx)
	if err := h.svc.SetPolicies(ctx, p, string(req.Body.EmailSalaryVisibility)); err != nil {
		return nil, err
	}
	v, err := h.svc.Policies(ctx, p)
	if err != nil {
		return nil, err
	}
	return api.PutPolicies200JSONResponse{EmailSalaryVisibility: api.EmailSalaryVisibility(v)}, nil
}
