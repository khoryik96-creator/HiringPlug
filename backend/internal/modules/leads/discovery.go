package leads

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const discoveryPage = 30

var discoverySources = []string{Greenhouse, Lever, Ashby, SmartRecruiters}

type discoveryInput struct {
	candidateID, documentID, target string
	countries, sources              []string
	cursor                          *string
}

func (s *Service) discover(ctx context.Context, p authz.Principal, in discoveryInput) (role string, rows []leadView, next *string, err error) {
	if !p.Can(api.PermissionCandidatesRead) || !p.Can(api.PermissionDocumentsRead) {
		return "", nil, nil, apperr.New(api.ErrorCodeFORBIDDEN, "Finding jobs from a CV needs permission to see candidates and their CVs.")
	}
	loc := locationPattern(in.countries)
	if loc == "" {
		return "", nil, nil, apperr.Validation(apperr.FieldErrors{{Field: "countries", Message: "Choose at least one supported country."}})
	}
	kinds := in.sources
	if len(kinds) == 0 {
		kinds = discoverySources
	}
	for _, kind := range kinds {
		if !slices.Contains(discoverySources, kind) {
			return "", nil, nil, apperr.Validation(apperr.FieldErrors{{Field: "sources", Message: "Choose supported job feeds."}})
		}
	}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		candidateID, source, cv, err := s.cvs.CVTextOf(ctx, tx, in.documentID)
		if database.IsNoRows(err) || candidateID != in.candidateID && err == nil {
			return apperr.NotFound("candidate CV")
		}
		if err != nil {
			return err
		}
		if source == "formatted" || len(cv) == 0 {
			return apperr.Validation(apperr.FieldErrors{{Field: "documentId", Message: "Choose an original CV with readable text."}})
		}
		var query string
		role, query = roleQuery(cv, in.target)
		if query == "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "targetRole", Message: "Enter a target job title."}})
		}
		batch, err := store.New(tx).LeadDiscoveryPage(ctx, store.LeadDiscoveryPageParams{
			SourceKinds: kinds, LocationRe: loc, TitleQuery: query, Cursor: in.cursor, RowLimit: discoveryPage + 1,
		})
		if err != nil {
			return err
		}
		batch, next = pageLeads(batch, discoveryPage, func(r store.LeadDiscoveryPageRow) string { return r.JobLead.ID })
		seen := map[string]bool{}
		for _, r := range batch {
			canonical := directFeedAd(*r.JobLead.SourceKind, *r.JobLead.Url, *r.JobLead.ExternalID) // SQL requires a known source and URL
			if canonical == "" || seen[canonical] {
				continue
			}
			seen[canonical] = true
			r.JobLead.Url = &canonical
			rows = append(rows, leadView{lead: r.JobLead, favorite: r.Favorite})
		}
		return nil
	})
	return role, rows, next, err
}
