package documents

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// saveExtraction handles document.text_extracted: the Python worker's result.
// It runs in a transaction scoped to the job's tenant.
func (s *Service) saveExtraction(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.DocumentTextExtractedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	return s.applyExtraction(ctx, tx, job.TenantID, result)
}

// readHere is the text of a file read here (text, RTF or ODT), as the AI
// worker would report it.
func readHere(id, contentType string, data []byte) api.DocumentTextExtractedJob {
	out := api.DocumentTextExtractedJob{DocumentId: id, Extractor: "hiringplug-" + strings.ToLower(filekind.Short(contentType)),
		Warnings: []string{}, PageCount: nullable.NewNullNullable[int](), Text: nullable.NewNullNullable[string](),
		ErrorMessage: nullable.NewNullNullable[string]()}
	text, err := filekind.Text(contentType, data)
	if err != nil {
		out.Status = api.ExtractionStatusFailed
		out.ErrorMessage = nullable.NewNullableWithValue(strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + ".")
		return out
	}
	out.Status = api.ExtractionStatusExtracted
	out.Text = nullable.NewNullableWithValue(text)
	return out
}

// applyExtraction saves a document's text (read by the AI worker, or here),
// then parses it and tells whoever waits for CVs to be read.
func (s *Service) applyExtraction(ctx context.Context, tx pgx.Tx, tenantID string, result api.DocumentTextExtractedJob) error {
	var text *string
	if t, err := result.Text.Get(); err == nil {
		cleaned := CleanExtractedText(t)
		text = &cleaned
	}
	var pages *int32
	if n, err := result.PageCount.Get(); err == nil {
		v := int32(n) //nolint:gosec // page counts are small
		pages = &v
	}
	var errMsg *string
	if m, err := result.ErrorMessage.Get(); err == nil {
		errMsg = &m
	}
	if result.Status != api.ExtractionStatusExtracted { // read again and it failed: the text read before is kept, and read
		before, err := store.New(tx).GetDocument(ctx, result.DocumentId)
		if err == nil && before.Status == string(api.DocumentStatusExtracted) && hasText(before.ExtractedText) {
			logx.From(ctx).Warn("a document's text could not be read again; the text read before is kept", "document_id", result.DocumentId)
			return s.queueParse(ctx, tx, tenantID, before)
		}
	}
	row, err := store.New(tx).SaveExtraction(ctx, store.SaveExtractionParams{
		ID: result.DocumentId, Status: string(result.Status), ExtractedText: text,
		PageCount: pages, Extractor: &result.Extractor, ErrorMessage: errMsg,
		ExtractionWarnings: nonNil(result.Warnings),
	})
	if database.IsNoRows(err) {
		logx.From(ctx).Info("document no longer exists; dropping its extraction result", "document_id", result.DocumentId)
		return nil
	}
	if err != nil {
		return err
	}
	// Every CV is parsed as soon as its text is saved: candidates' CVs for
	// review, formatted ones (our own output) for comparing with the original.
	if row.Status == string(api.DocumentStatusExtracted) && hasText(row.ExtractedText) {
		if err := s.queueParse(ctx, tx, tenantID, row); err != nil {
			return err
		}
		if row.Source != "formatted" {
			for _, hook := range s.whenRead {
				if err := hook(ctx, tx, tenantID, row.CandidateID); err != nil {
					return err
				}
			}
		}
	} else {
		// No text: nothing to parse. A reading asked for again ends here too
		// (back to the details read before, or failed), not "being read" forever.
		if row.ParseStatus != nil && *row.ParseStatus == string(api.DocumentParseStatusQueued) {
			if _, err := store.New(tx).EndParse(ctx, store.EndParseParams{ID: row.ID, FailedError: noDetails, KeptWarning: keptWarning}); err != nil {
				return err
			}
		}
		if err := s.readDone(ctx, tx, tenantID, row.ID, nil); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "document.text_extracted", EntityType: "document", EntityID: result.DocumentId,
		Changes: map[string]any{"status": result.Status, "extractor": result.Extractor, "warnings": len(result.Warnings)},
	})
}

// queueParse asks the AI worker to parse a document's saved text; the AI may
// fill in what the rules leave empty when it is on and allowance is left.
func (s *Service) queueParse(ctx context.Context, tx pgx.Tx, tenantID string, row store.Document) error {
	useAI := false
	var ai *api.AiAccess
	if s.ai != nil {
		var err error
		if ai, useAI, err = s.ai.Allowed(ctx, tx, tenantID, "cv_reading"); err != nil {
			return err
		}
	}
	// The reading's own key comes back with its result: its AI use counts once.
	runKey := rand.Text()
	job := api.DocumentParseCvJob{DocumentId: row.ID, Text: *row.ExtractedText, UseAi: &useAI, RunKey: &runKey}
	if row.Source != "formatted" { // often the candidate's name; a formatted CV's starts with the agency's
		name := row.FileName
		job.FileName = &name
	}
	if useAI {
		job.Ai = ai
		forAI := redact.WithoutSalary(*row.ExtractedText) // salary never goes to the AI
		job.AiText = &forAI
		if s.industryLabels != nil {
			labels, err := s.industryLabels(ctx, tx)
			if err != nil {
				return err
			}
			job.Industries = &labels
		}
	}
	if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentParseCv, job); err != nil {
		return err
	}
	_, err := store.New(tx).QueueParse(ctx, row.ID)
	return err
}

// saveParse handles document.cv_parsed: the parser's result. Only values
// whose quote is in the saved CV text are kept (groundProfile).
func (s *Service) saveParse(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.DocumentCvParsedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.GetDocument(ctx, result.DocumentId)
	if database.IsNoRows(err) {
		logx.From(ctx).Info("document no longer exists; dropping its parse result", "document_id", result.DocumentId)
		if result.AiTokens == nil || result.RunKey == nil || *result.RunKey == "" {
			return nil
		}
		// What the AI used is still counted, once per reading.
		return s.recordAI(ctx, tx, job, store.Document{ID: result.DocumentId}, result.RunKey, int64(*result.AiTokens))
	}
	if err != nil {
		return err
	}
	status := string(api.DocumentParseStatusFailed)
	params := store.SaveParseParams{ID: row.ID, ParseStatus: &status, ParseError: parseError(result)}
	changes := map[string]any{"status": status}
	if row.ParsedProfile != nil { // read again and it failed: the details read before stay, with why as a warning
		status = string(api.DocumentParseStatusParsed)
		params.ParsedProfile, params.Parser, params.ParseError = row.ParsedProfile, row.Parser, keptError(result)
		changes = map[string]any{"status": "failed", "kept": true}
	}
	var kept *api.ParsedProfile
	if profile, err := result.Profile.Get(); err == nil && hasText(row.ExtractedText) {
		grounded := groundProfile(*row.ExtractedText, profile)
		if s.enrichIndustry != nil {
			if err := s.enrichIndustry(ctx, tx, &grounded); err != nil {
				return err
			}
		}
		kept = &grounded
		data, err := json.Marshal(grounded)
		if err != nil {
			return err
		}
		status = string(api.DocumentParseStatusParsed)
		params.ParsedProfile, params.Parser, params.ParseError = data, &grounded.Parser, nil
		// Counts only: CV contents never go in the audit log.
		changes = map[string]any{
			"status": status, "parser": grounded.Parser, "jobs": len(grounded.Experience),
			"skills": len(grounded.Skills), "dropped": grounded.Dropped,
		}
	}
	// The last result to land is kept, so a CV read again while an earlier
	// reading ran ends with the reading asked for last. Its AI use counts once
	// per reading (by its key), however often the result is delivered.
	if _, err := q.SaveParse(ctx, params); err != nil {
		return err
	}
	if result.AiTokens != nil && *result.AiTokens > 0 {
		tokens := min(*result.AiTokens, 1<<30)
		if err := q.RecordAIUse(ctx, store.RecordAIUseParams{DocumentID: row.ID, Tokens: int32(tokens)}); err != nil { //nolint:gosec // capped
			return err
		}
		if err := s.recordAI(ctx, tx, job, row, result.RunKey, int64(tokens)); err != nil {
			return err
		}
		changes["aiTokens"] = *result.AiTokens
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "document.cv_parsed", EntityType: "document", EntityID: row.ID, Changes: changes,
	}); err != nil {
		return err
	}
	return s.readDone(ctx, tx, job.TenantID, row.ID, kept)
}

// recordAI counts a reading's AI tokens once (by the run key its parse job
// carried; a result queued before run keys existed uses its own job's ID),
// for whoever added the CV.
func (s *Service) recordAI(ctx context.Context, tx pgx.Tx, job jobqueue.Job, row store.Document, runKey *string, tokens int64) error {
	if s.ai == nil {
		return nil
	}
	key := job.ID
	if runKey != nil && *runKey != "" {
		key = *runKey
	}
	var who *string // nil: the document is gone
	if row.CreatedBy != "" {
		who = &row.CreatedBy
	}
	return s.ai.RecordAI(ctx, tx, job.TenantID, "cv_reading", row.ID, key, who, tokens, 0)
}

// readDone runs the hooks for a document whose reading has ended.
func (s *Service) readDone(ctx context.Context, tx pgx.Tx, tenantID, documentID string, profile *api.ParsedProfile) error {
	for _, hook := range s.whenDone {
		if err := hook(ctx, tx, tenantID, documentID, profile); err != nil {
			return err
		}
	}
	return nil
}

func hasText(t *string) bool {
	return t != nil && strings.TrimSpace(*t) != ""
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// keptError is why a reading again read nothing, as a warning shown with
// the details read before (at most 300 characters, as the column allows).
func keptError(result api.DocumentCvParsedJob) *string {
	msg := "The CV could not be read again, so the details read before are kept: " + *parseError(result)
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300])
	}
	return &msg
}

// parseError is why the parser read nothing (its error type and where it
// broke; never CV text), at most 300 characters as the column allows.
func parseError(result api.DocumentCvParsedJob) *string {
	msg, err := result.ErrorMessage.Get()
	if err != nil || strings.TrimSpace(msg) == "" {
		msg = "The details could not be read from this CV."
	}
	if r := []rune(strings.TrimSpace(msg)); len(r) > 300 {
		msg = string(r[:300])
	}
	return &msg
}
