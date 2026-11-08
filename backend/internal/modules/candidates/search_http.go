package candidates

import (
	"context"
	"strings"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// SearchCandidates handles POST /v1/candidates/search.
func (h *HTTP) SearchCandidates(ctx context.Context, req api.SearchCandidatesRequestObject) (api.SearchCandidatesResponseObject, error) {
	b := req.Body
	p := authz.MustFromContext(ctx)
	limit := defaultPageSize
	if b.Limit != nil {
		limit = *b.Limit
	}
	f := FilterOf(*b, p)
	page, err := h.svc.Search(ctx, p, f, b.Cursor, limit)
	if err != nil {
		return nil, err
	}
	out := api.SearchCandidates200JSONResponse{Items: make([]api.CandidateSearchHit, len(page.Items)), NextCursor: orNull(page.NextCursor), Terms: page.Terms}
	if out.Terms == nil {
		out.Terms = []string{}
	}
	for i, hit := range page.Items {
		where := []api.CandidateSearchHitMatchedIn{}
		if hit.InCV {
			where = append(where, api.CandidateSearchHitMatchedInCv)
		}
		if hit.InProfile {
			where = append(where, api.CandidateSearchHitMatchedInProfile)
		}
		out.Items[i] = api.CandidateSearchHit{Candidate: summaryToAPI(hit.Summary), MatchedIn: where, CvSnippet: nullable.NewNullNullable[string]()}
		if hit.CVSnippet != nil {
			out.Items[i].CvSnippet = nullable.NewNullableWithValue(*hit.CVSnippet)
		}
	}
	return out, nil
}

func blankNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

// strs is a list of enum or plain strings as plain strings (nil: none).
func strs[T ~string](in *[]T) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(*in))
	for i, v := range *in {
		out[i] = string(v)
	}
	return out
}

// FilterOf is an advanced search as the API takes it ("me" as owner is the user).
func FilterOf(b api.CandidateSearch, p authz.Principal) SearchFilter {
	f := SearchFilter{
		Query: blankNil(b.Q), Boolean: blankNil(b.Boolean), Location: blankNil(b.Location),
		Availability: strs(b.Availability), WorkKinds: strs(b.WorkAuthorization), Nationality: strs(b.Nationality),
		Residence: strs(b.Residence), Sources: strs(b.Source), Industries: strs(b.Industry), Skills: strs(b.Skills), Tags: strs(b.Tags),
		AllSkills: b.SkillsMatch == nil || *b.SkillsMatch == "all", NoticeMaxDays: b.NoticeMaxDays, UpdatedWithinDays: b.UpdatedWithinDays,
		Owner: blankNil(b.Owner),
	}
	if f.Owner != nil && *f.Owner == "me" {
		f.Owner = &p.UserID
	}
	if s := b.ExpectedSalaryMax; s != nil {
		amount, currency := s.Amount, s.Currency
		f.SalaryMax, f.SalaryCurrency = &amount, &currency
	}
	return f
}
