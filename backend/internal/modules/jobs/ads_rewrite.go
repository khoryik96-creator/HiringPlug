package jobs

// AI rewrites of an ad's blind version, CV Studio's way: the AI worker
// paraphrases the ad without the client's identity and runs CV Studio's JD
// checks (jd.blind); the result comes back on jd.blinded and becomes the
// ad's blind version, as if a person had written it.

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// AI is the company's AI provider, if it has one and allowance left this month.
type AI interface {
	Allowed(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error)
}

// RewriteOptions are how a blind ad is rewritten.
type RewriteOptions struct {
	Tone, Strength                 string // CV Studio's: professional / conversational / concise; low / medium / high
	RemoveAlso, WhyJoinUs, Context *string
}

// BlindRewrite is a rewrite run and, once done, how it went.
type BlindRewrite = store.JobAdBlindRewrite

// StartBlindRewrite queues rewriting an ad's blind version with the AI.
func (s *Service) StartBlindRewrite(ctx context.Context, p authz.Principal, adID string, o RewriteOptions) (BlindRewrite, error) {
	var out BlindRewrite
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getAdRow(ctx, store.New(tx), adID)
		if err != nil {
			return err
		}
		out, err = s.startRewrite(ctx, tx, p, adFromRow(row), o, false)
		return err
	})
	return out, err
}

// AutoRewrite is how a new ad's blind version is rewritten on its own.
var AutoRewrite = RewriteOptions{Tone: "professional", Strength: "medium"}

// startRewrite queues the AI rewrite of ad's blind version in tx. With
// auto (a new ad, blinded on its own as CV Studio does on upload), an ad
// too short to rewrite, or a company without the AI, is left to the
// blind version made by the rules: no rewrite, no error.
func (s *Service) startRewrite(ctx context.Context, tx pgx.Tx, p authz.Principal, ad Ad, o RewriteOptions, auto bool) (BlindRewrite, error) {
	q := store.New(tx)
	raw := RewriteSource(ad.Details)
	if utf8.RuneCountInString(raw) < MinRewriteText {
		if auto {
			return BlindRewrite{}, nil
		}
		return BlindRewrite{}, apperr.New(api.ErrorCodeJOBADBLINDTOOSHORT, "Write the ad first: there is too little text to rewrite.")
	}
	var access *api.AiAccess
	on := false
	if s.ai != nil {
		var err error
		if access, on, err = s.ai.Allowed(ctx, tx); err != nil {
			return BlindRewrite{}, err
		}
	}
	if !on {
		if auto {
			return BlindRewrite{}, nil
		}
		return BlindRewrite{}, apperr.New(api.ErrorCodeJOBADBLINDAIOFF,
			"Rewriting needs the AI: set up an AI provider in Settings, or wait for next month's allowance.")
	}
	client, err := s.clientOf(ctx, tx, ad.JobID)
	if err != nil {
		return BlindRewrite{}, err
	}
	blind := Blind(ad.Details, client)
	out, err := q.InsertBlindRewrite(ctx, store.InsertBlindRewriteParams{
		AdID: ad.ID, Tone: o.Tone, Strength: o.Strength, SourceHash: strPtr(SourceHash(ad.Details)), // the ad it is made from
	})
	if err != nil {
		return BlindRewrite{}, err
	}
	if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindJdBlind, api.JdBlindJob{
		RewriteId: out.ID, Raw: raw, CompanyName: client.Name, Industry: nullable.NewNullNullable[string](),
		Location: orNull(blind.Location), WorkMode: orNull(strPtr(WorkMode(ad.Details))),
		ClientDescription: orNull(ad.Details.ClientDescription), Context: orNull(o.Context),
		RemoveAlso: orNull(strPtr(RemoveAlso(o.RemoveAlso, client))), WhyJoinUs: orNull(o.WhyJoinUs),
		Tone: api.BlindRewriteTone(o.Tone), Strength: api.BlindRewriteStrength(o.Strength), Ai: access,
	}); err != nil {
		return BlindRewrite{}, err
	}
	return out, audit.Record(ctx, tx, audit.Entry{Action: "job_ad.blind_rewrite_started", EntityType: "job_ad", EntityID: ad.ID, Changes: map[string]any{
		"rewriteId": out.ID, "tone": o.Tone, "strength": o.Strength, "auto": auto,
	}})
}

// ListBlindRewrites returns an ad's latest rewrites, newest first.
func (s *Service) ListBlindRewrites(ctx context.Context, p authz.Principal, adID string) ([]BlindRewrite, error) {
	var out []BlindRewrite
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := getAdRow(ctx, q, adID); err != nil {
			return err
		}
		var err error
		out, err = q.ListBlindRewrites(ctx, adID)
		return err
	})
	return expireStale(out, time.Now()), err
}

// RewriteGivesUpAfter is how long a rewrite may stay queued before it is
// shown as failed: its AI job died (the worker stopped, or every attempt
// failed) and no answer will come. An answer that still arrives is used.
const RewriteGivesUpAfter = 30 * time.Minute

// expireStale shows rewrites queued for too long as failed, so the ad can be
// rewritten again.
func expireStale(runs []BlindRewrite, now time.Time) []BlindRewrite {
	for i, r := range runs {
		if r.Status == "queued" && now.Sub(r.CreatedAt) > RewriteGivesUpAfter {
			reason := "It took too long: the AI did not answer. Try again."
			runs[i].Status, runs[i].Error = "failed", &reason
		}
	}
	return runs
}

// saveBlindRewrite handles jd.blinded: the AI's blind job description
// becomes the ad's blind version, unless it still names the client.
func (s *Service) saveBlindRewrite(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.JdBlindedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	run, err := q.GetBlindRewrite(ctx, r.RewriteId)
	if database.IsNoRows(err) || (err == nil && run.Status != "queued") {
		return nil // a repeated result, or the ad is gone
	}
	if err != nil {
		return err
	}
	warnings := r.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	finish := func(status string, reason *string) error {
		model, _ := r.Model.Get()
		_, err := q.FinishBlindRewrite(ctx, store.FinishBlindRewriteParams{
			ID: run.ID, Status: status, Error: reason, Warnings: warnings, Retried: r.Retried,
			Model: nonEmpty(model), Tokens: int32(min(r.InputTokens+r.OutputTokens, 1<<30)), //nolint:gosec // bounded above
		})
		return err
	}
	jd, err := r.Jd.Get()
	if err != nil { // no rewrite: why
		reason := "It could not be rewritten."
		if e, err := r.Error.Get(); err == nil && e != "" {
			reason = clipText("It could not be rewritten: "+e, 2000)
		}
		return finish("failed", &reason)
	}
	row, err := getAdRow(ctx, q, run.AdID)
	if err != nil {
		return err
	}
	ad := adFromRow(row)
	client, err := s.clientOf(ctx, tx, ad.JobID)
	if err != nil {
		return err
	}
	title, text := RewriteText(jd, Blind(ad.Details, client).Title)
	if NamesClient(title, client) || NamesClient(text, client) { // never shown to candidates
		reason := "The AI's version still names the client, so it was not used. Try again, or edit the blind ad yourself."
		return finish("failed", &reason)
	}
	hash := SourceHash(ad.Details)
	if run.SourceHash != nil { // the ad as it was sent: later changes still mark the blind version out of date
		hash = *run.SourceHash
	}
	if _, err := q.SetAdBlindText(ctx, store.SetAdBlindTextParams{
		ID: ad.ID, BlindTitleEdit: &title, BlindDescriptionEdit: nonEmpty(text), BlindSourceHash: &hash,
	}); err != nil {
		return err
	}
	if err := finish("done", nil); err != nil {
		return err
	}
	logx.From(ctx).Info("blind ad rewritten", "rewrite_id", run.ID, "ad_id", ad.ID, "warnings", len(warnings))
	return audit.Record(ctx, tx, audit.Entry{Action: "job_ad.blind_rewritten", EntityType: "job_ad", EntityID: ad.ID, Changes: map[string]any{
		"rewriteId": run.ID, "warnings": len(warnings), "retried": r.Retried,
	}})
}

// TokensSince is the AI tokens blind ad rewrites used since a time (the
// company's monthly allowance), in the caller's transaction.
func (s *Service) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).BlindRewriteTokensSince(ctx, since)
}

func orNull(s *string) nullable.Nullable[string] {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(strings.TrimSpace(*s))
}

func strPtr(s string) *string { return &s }

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
