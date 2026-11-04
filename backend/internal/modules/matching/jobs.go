package matching

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// noRequirements is why a match has no score when none of the AI's
// requirements quote the job.
const noRequirements = "None of the requirements the AI listed could be found in the job's text, so there is no score."

// scored handles match.scored: the AI worker's reading of the job and the
// CV, or why there is none. Every quote is checked against what was sent,
// the score is worked out from what passes, and what was sent is cleared.
func (s *Service) scored(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.MatchScoredJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	sent, err := q.GetQueuedForUpdate(ctx, r.MatchId)
	if database.IsNoRows(err) {
		return nil // already saved (a repeated result): nothing to do
	}
	if err != nil {
		return err
	}
	params := store.SaveScoreParams{
		ID: r.MatchId, Status: "failed", Model: clipped(apiconv.ValueOrNil(r.Model), 200),
		InputTokens: int32(min(max(r.InputTokens, 0), 1<<30)), OutputTokens: int32(min(max(r.OutputTokens, 0), 1<<30)), //nolint:gosec // capped
	}
	var reasons []Reason
	if e := apiconv.ValueOrNil(r.Error); e != nil && strings.TrimSpace(*e) != "" {
		params.Error = clipped(e, 1000)
	} else {
		claims := make([]Claim, len(r.Claims))
		for i, c := range r.Claims {
			claims[i] = Claim{Requirement: c.Requirement, Verdict: string(c.Verdict), Evidence: apiconv.ValueOrNil(c.Evidence), Note: apiconv.ValueOrNil(c.Note)}
		}
		var dropped int
		reasons, dropped = Check(deref(sent.SentJob), deref(sent.SentCv), claims)
		params.Dropped = int32(min(dropped, 1<<30)) //nolint:gosec // capped
		if score, ok := Score(reasons); ok {
			v := int32(score) //nolint:gosec // 0 to 100
			params.Status, params.Score, params.Summary = "done", &v, Summary(apiconv.ValueOrNil(r.Summary))
		} else {
			params.Error = ptr(noRequirements)
		}
	}
	if reasons == nil {
		reasons = []Reason{}
	}
	if params.Reasons, err = json.Marshal(reasons); err != nil {
		return err
	}
	if n, err := q.SaveScore(ctx, params); err != nil || n == 0 {
		return err
	}
	// No quotes or notes in the audit log: they are about a person.
	changes := map[string]any{"status": params.Status, "requirements": len(reasons), "dropped": params.Dropped, "tokens": params.InputTokens + params.OutputTokens}
	if params.Score != nil {
		changes["score"] = *params.Score
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "match.scored", EntityType: "match", EntityID: r.MatchId, Changes: changes})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
