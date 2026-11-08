package matching

// AI candidate search, CV Studio's AI Crawler on the company's own
// candidates: the boolean and filters choose who may be considered, a job
// description orders them, the first ones are sent to the AI (CVs without
// names, contact details or salary; none who withdrew consent), and the
// AI's ranking comes back on talent.ranked.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// StartTalentSearch finds the candidates to consider and asks the AI to rank them.
func (s *Service) StartTalentSearch(ctx context.Context, p authz.Principal, in api.TalentSearchInput) (store.TalentSearch, error) {
	jd := ""
	if in.Jd != nil {
		jd = strings.TrimSpace(*in.Jd)
	}
	boolean := ""
	if in.Search.Boolean != nil {
		boolean = strings.TrimSpace(*in.Search.Boolean)
	}
	if jd == "" && boolean == "" {
		return store.TalentSearch{}, apperr.Validation(apperr.FieldErrors{{Field: "jd", Message: "add a job description, or a boolean to search with"}})
	}
	if !p.Can(api.PermissionCandidatesRead) || !p.Can(api.PermissionDocumentsRead) {
		return store.TalentSearch{}, apperr.New(api.ErrorCodeFORBIDDEN, "An AI candidate search reads candidates' CVs, which you may not see.")
	}
	rank := TalentRankDefault
	if in.Rank != nil {
		rank = *in.Rank
	}
	strict := in.Strict != nil && *in.Strict
	var out store.TalentSearch
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ai, err := s.on(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.withinAllowance(ctx, tx); err != nil {
			return err
		}
		pool, err := s.deps.Candidates.Pool(ctx, tx, p, in.Search, TalentPool)
		if err != nil {
			return err
		}
		order := pool
		if jd != "" {
			ids := make([]string, len(pool))
			for i, c := range pool {
				ids[i] = c.ID
			}
			ranked, err := s.deps.CVs.RankCVsByText(ctx, tx, jd, ids, TalentPool)
			if err != nil {
				return err
			}
			order = TalentOrder(pool, ranked)
		}
		var sent []string
		var cands []api.TalentCandidate
		for _, c := range order {
			if len(sent) == rank {
				break
			}
			if no, err := s.consentWithdrawn(ctx, tx, c.ID); err != nil || no {
				if err != nil {
					return err
				}
				continue
			}
			cv, err := s.cvText(ctx, tx, p, c.ID)
			if err != nil {
				return err
			}
			cands = append(cands, api.TalentCandidate{Ref: Ref(len(sent)), Profile: clip(c.Profile, 2000), Cv: clip(cv, talentCVText)})
			sent = append(sent, c.ID)
		}
		filters, err := json.Marshal(in.Search)
		if err != nil {
			return err
		}
		params := store.InsertTalentSearchParams{
			Status: "queued", Jd: nonBlank(jd), Boolean: nonBlank(boolean), Filters: filters, Strict: strict,
			Considered: int32(len(pool)), Sent: orNone(sent), //nolint:gosec // at most TalentPool
		}
		if len(sent) == 0 {
			none := "No candidates match the boolean and filters (or those who do withdrew consent), so nothing was sent to the AI. Loosen the search and try again."
			params.Status, params.Summary = "done", &none
		}
		if out, err = store.New(tx).InsertTalentSearch(ctx, params); err != nil {
			return err
		}
		if len(sent) > 0 {
			if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindTalentRank, api.TalentRankJob{
				SearchId: out.ID, Requirements: clip(redact.WithoutSalary(TalentRequirements(jd, in.Search)), 24000),
				Strict: strict, Candidates: cands, Ai: ai,
			}); err != nil {
				return err
			}
		}
		// Counts only: the search may hold a salary cap, never written to the audit log.
		return audit.Record(ctx, tx, audit.Entry{Action: "talent_search.started", EntityType: "talent_search", EntityID: out.ID,
			Changes: map[string]any{"withJd": jd != "", "withBoolean": boolean != "", "considered": len(pool), "sent": len(sent)}})
	})
	return out, err
}

// TalentSearch returns one of the user's searches, and its ranking.
func (s *Service) TalentSearch(ctx context.Context, p authz.Principal, id string) (api.TalentSearch, error) {
	var out api.TalentSearch
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetTalentSearch(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("talent search")
		}
		if err != nil {
			return err
		}
		out, err = s.talentToAPI(ctx, tx, row)
		return err
	})
	return out, err
}

// TalentSearches returns the user's latest searches.
func (s *Service) TalentSearches(ctx context.Context, p authz.Principal) ([]api.TalentSearch, error) {
	out := []api.TalentSearch{}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListTalentSearches(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			v, err := s.talentToAPI(ctx, tx, r)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func (s *Service) talentToAPI(ctx context.Context, tx pgx.Tx, r store.TalentSearch) (api.TalentSearch, error) {
	var saved []TalentResult
	if err := json.Unmarshal(r.Results, &saved); err != nil {
		return api.TalentSearch{}, err
	}
	ids := make([]string, len(saved))
	for i, x := range saved {
		ids[i] = x.CandidateID
	}
	briefs := map[string]CandidateBrief{}
	if len(ids) > 0 {
		var err error
		if briefs, err = s.deps.Candidates.Briefs(ctx, tx, ids); err != nil { // only those the user may see
			return api.TalentSearch{}, err
		}
	}
	out := api.TalentSearch{
		Id: r.ID, Status: api.TalentSearchStatus(r.Status), Jd: apiconv.OrNull(r.Jd), Boolean: apiconv.OrNull(r.Boolean),
		Strict: r.Strict, Considered: int(r.Considered), Sent: len(r.Sent), Error: apiconv.OrNull(r.Error),
		Summary: apiconv.OrNull(r.Summary), MustHaves: orNone(r.MustHaves), NiceToHaves: orNone(r.NiceToHaves),
		SuggestedBoolean: apiconv.OrNull(r.SuggestedBoolean), Results: []api.TalentFit{}, CreatedAt: r.CreatedAt,
		FinishedAt: nullable.NewNullNullable[time.Time](),
	}
	if r.FinishedAt != nil {
		out.FinishedAt = nullable.NewNullableWithValue(*r.FinishedAt)
	}
	if r.Status == "queued" && s.now().Sub(r.CreatedAt) > TalentGivesUpAfter {
		out.Status = api.TalentSearchStatusFailed
		out.Error = nullable.NewNullableWithValue("It took too long: the AI did not answer. Try again.")
	}
	for _, x := range saved {
		b, ok := briefs[x.CandidateID]
		if !ok {
			continue
		}
		out.Results = append(out.Results, api.TalentFit{
			CandidateId: x.CandidateID, Name: b.Name, CurrentTitle: apiconv.OrNull(b.Title), Fit: x.Fit,
			Verdict: api.TalentVerdict(x.Verdict), Reasons: orNone(x.Reasons), Gaps: orNone(x.Gaps),
		})
	}
	return out, nil
}

// talentRanked handles talent.ranked: the AI's ranking, checked against who
// it was sent, and its notes.
func (s *Service) talentRanked(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.TalentRankedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.TalentSearchForAnswer(ctx, r.SearchId)
	if database.IsNoRows(err) || (err == nil && row.Status != "queued") {
		return nil // a repeated answer, or the search is gone
	}
	if err != nil {
		return err
	}
	status := "done"
	results := TalentResults(row.Sent, r.Results)
	reason, _ := r.Error.Get()
	var failure *string
	if reason = strings.TrimSpace(reason); reason != "" {
		status = "failed"
		msg := clip("The AI could not rank them: "+reason, 2000)
		failure = &msg
	}
	body, err := json.Marshal(results)
	if err != nil {
		return err
	}
	summary, _ := r.Summary.Get()
	suggested, _ := r.SuggestedBoolean.Get()
	model, _ := r.Model.Get()
	if _, err := q.FinishTalentSearch(ctx, store.FinishTalentSearchParams{
		ID: row.ID, Status: status, Results: body, Summary: nonBlank(clip(summary, 3000)),
		MustHaves: cleanList(r.MustHaves), NiceToHaves: cleanList(r.NiceToHaves), SuggestedBoolean: nonBlank(clip(suggested, 1000)),
		Error: failure, Model: nonBlank(clip(model, 200)),
		Tokens: int32(min(max(r.InputTokens, 0)+max(r.OutputTokens, 0), 1<<30)), //nolint:gosec // capped
	}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "talent_search.ranked", EntityType: "talent_search", EntityID: row.ID,
		Changes: map[string]any{"status": status, "ranked": len(results)}})
}

func nonBlank(s string) *string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &s
}

func orNone[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
