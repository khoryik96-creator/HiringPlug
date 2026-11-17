package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Searches must be able to use their indexes under row-level security: as
// the app's role, inside an agency's transaction. Row-level security keeps
// conditions that are not LEAKPROOF away from indexes, and every boolean
// search then read all the CVs (seconds each with half a million CVs).
// The agency gets a few thousand records, so that a rare word's index beats
// reading the agency's rows.
func TestSearchesCanUseTheirIndexes(t *testing.T) {
	t.Parallel()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	p := authz.NewPrincipalWithAccess(tenant.ID, user.ID, "", "", api.RoleRecruiter, authz.PermissionsForRole(api.RoleRecruiter), authz.Scope{Users: []string{user.ID}})
	seed := []string{
		`INSERT INTO candidate (tenant_id, first_name, last_name, phone, tags, skills, current_title, created_by)
		 SELECT $1, 'Synthetic ' || g, 'Person' || g, '+6011' || lpad(g::text, 7, '0'), ARRAY['tag' || g], ARRAY['skill' || g], 'Engineer', $2
		 FROM generate_series(1, 3000) g`,
		`INSERT INTO document (tenant_id, candidate_id, source, file_name, content_type, size_bytes, sha256, storage_key, status, extracted_text, created_by)
		 SELECT tenant_id, id, 'original', 'cv.pdf', 'application/pdf', 1, encode(sha256(convert_to(id, 'UTF8')), 'hex'), tenant_id || '/' || id, 'extracted', 'Engineer with plenty of experience ' || id, $2
		 FROM candidate WHERE tenant_id = $1`,
		`INSERT INTO company (tenant_id, name, created_by) SELECT $1, 'Synthetic company ' || g, $2 FROM generate_series(1, 3000) g`,
		`INSERT INTO contact (tenant_id, company_id, first_name, last_name, title, created_by)
		 SELECT tenant_id, id, 'Synthetic', 'Contact', 'Engineer', $2 FROM company WHERE tenant_id = $1`,
		`ANALYZE candidate, document, cv_search, company, contact`,
	}
	for _, q := range seed {
		args := []any{tenant.ID, user.ID}
		if strings.HasPrefix(q, "ANALYZE") {
			args = nil
		}
		_, err := testutil.Current.Admin.Exec(context.Background(), q, args...)
		require.NoError(t, err, q)
	}
	cases := []struct{ index, table, where string }{
		{"cv_search_terms_idx", "cv_search", `terms @@ to_tsquery('english', 'java & python')`},
		{"candidate_profile_terms_idx", "candidate", `profile_terms @@ to_tsquery('english', 'java')`},
		{"candidate_search_trgm_idx", "candidate", `search_text LIKE '%tanaka%'`},
		{"candidate_name_trgm_idx", "candidate", `app_normalize_text('tanaka') <% name_text`},
		{"candidate_phone_trgm_idx", "candidate", `phone_digits LIKE '%601234%'`},
		{"candidate_skills_idx", "candidate", `app_lower_array(skills) @> ARRAY['java']`},
		{"candidate_tags_idx", "candidate", `app_lower_array(tags) && ARRAY['vip']`},
		{"company_search_doc_idx", "company", `app_search_doc(name, website, industry, industry_terms, industry_hint, city, about) @@ to_tsquery('english', 'bank')`},
		{"company_search_trgm_idx", "company", `search_text LIKE '%bank%'`},
		{"contact_search_doc_idx", "contact", `app_search_doc(first_name || ' ' || last_name, title, email) @@ to_tsquery('english', 'manager')`},
		{"contact_name_trgm_idx", "contact", `name_text LIKE '%tanaka%'`},
	}
	ctx := context.Background()
	explain := func(q pgx.Tx, query string, args ...any) string {
		_, err := q.Exec(ctx, "SET LOCAL enable_seqscan = off")
		require.NoError(t, err)
		rows, err := q.Query(ctx, "EXPLAIN "+query, args...)
		require.NoError(t, err)
		lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
		require.NoError(t, err)
		return strings.Join(lines, "\n")
	}
	used := 0
	for _, c := range cases {
		// The admin is not held back by row-level security: where its plan
		// for the agency's rows uses the index, the app's must too.
		admin, err := testutil.Current.Admin.Begin(ctx)
		require.NoError(t, err)
		adminPlan := explain(admin, "SELECT 1 FROM "+c.table+" WHERE tenant_id = $1 AND "+c.where, tenant.ID)
		require.NoError(t, admin.Rollback(ctx))
		var appPlan string
		require.NoError(t, database.InTx(ctx, testutil.Current.App, p, func(tx pgx.Tx) error {
			appPlan = explain(tx, "SELECT 1 FROM "+c.table+" WHERE "+c.where)
			return nil
		}))
		if strings.Contains(adminPlan, c.index) {
			used++
			assert.Contains(t, appPlan, c.index, "%s\n%s", c.where, appPlan)
		}
	}
	assert.GreaterOrEqual(t, used, len(cases)/2, "most of the searches should pick their index here")
}
