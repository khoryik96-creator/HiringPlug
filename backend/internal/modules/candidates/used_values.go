package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// UsedValues are tags, skills, industries and specializations already used in
// the company, most used first.
type UsedValues struct {
	Tags, Skills, Industries, Specializations []string
}

// Used returns the values already used, for suggestions.
func (s *Service) Used(ctx context.Context, p authz.Principal) (UsedValues, error) {
	out := UsedValues{Tags: []string{}, Skills: []string{}, Industries: []string{}, Specializations: []string{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).UsedValues(ctx)
		for _, r := range rows {
			switch r.Kind {
			case "tag":
				out.Tags = append(out.Tags, r.Value)
			case "skill":
				out.Skills = append(out.Skills, r.Value)
			case "industry":
				out.Industries = append(out.Industries, r.Value)
			case "specialization":
				out.Specializations = append(out.Specializations, r.Value)
			}
		}
		return err
	})
	return out, err
}
