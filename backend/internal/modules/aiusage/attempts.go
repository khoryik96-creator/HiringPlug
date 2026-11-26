package aiusage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// An AI job's result records what it spent, but an attempt that fails after
// the AI answered (a reply that could not be read, a worker that stopped)
// spent tokens the provider charged, and its retry spends them again. The
// AI worker reports such an attempt (ai.attempt_spent) and it is recorded
// here under the job's feature, once per attempt (the job and its attempt
// number are the ledger key), so the month's use is the provider's bill.

// featureOfJob is the feature each AI job kind spends for.
var featureOfJob = map[api.JobKind]string{
	api.JobKindAssistantAsk: "assistant", api.JobKindAssistantEmail: "assistant_email", api.JobKindMatchScore: "fit_score",
	api.JobKindTalentRank: "talent_search", api.JobKindDocumentParseCv: "cv_reading", api.JobKindCvFormatStudio: "cv_studio",
	api.JobKindJdBlind: "jd_blind", api.JobKindJobReadJd: "jd_reading", api.JobKindCompanyIndustry: "industry_pick",
	api.JobKindInterviewPrepWrite: "interview_prep", api.JobKindAiproviderCheck: "provider_check",
}

// FeatureOfJob is the feature an AI job kind spends for ("" for one that
// does not ask the AI).
func FeatureOfJob(kind api.JobKind) string { return featureOfJob[kind] }

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAiAttemptSpent: m.service.attemptSpent}
}

func (s *Service) attemptSpent(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var in api.AiAttemptSpentJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	feature := FeatureOfJob(api.JobKind(in.JobKind))
	if feature == "" {
		logx.From(ctx).Warn("ai usage: an attempt of a job kind that does not ask the AI", "job_kind", in.JobKind)
		return nil
	}
	_, err := s.record(ctx, tx, job.TenantID, Use{
		Feature: feature, SourceID: in.JobId, RunKey: fmt.Sprintf("attempt-%d", in.Attempt),
		Input: in.InputTokens, Output: in.OutputTokens,
	})
	return err
}
