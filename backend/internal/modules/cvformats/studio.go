package cvformats

// Studio CVs: CV Studio's way of formatting. One of the candidate's own CVs is
// read by the AI and laid out by CV Studio's rules and Word renderer in the
// AI worker (cv.format_studio); the result comes back on cv.studio_formatted
// and is saved with the candidate's documents (and turned into a PDF).

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// SourceCV is a document a studio CV can be made from (documents module).
type SourceCV struct {
	ID, CandidateID, Source, FileName, ContentType string
	SizeBytes                                      int64
}

// formattable are the CVs CV Studio reads: PDF and Word.
var formattable = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/msword": true,
}

// StudioOptions are how a studio CV is made.
type StudioOptions struct {
	DocumentID                            string
	Blind, NeutralGender, CorrectLanguage bool
	Alignment                             string // left or justify
	Summary                               string // the About Him / Her summary: none, short, normal or detailed
	SummaryFocus                          string // general, technology, leadership or commercial
	SummaryDirection                      string // the recruiter's direction and tone, in their words
}

// summaryOptions settles the summary's options: a blind CV keeps its own
// summary; no summary, no focus or direction; a direction is one tidy line,
// and never about pay (pay never goes to the AI, and the summary never says it).
func summaryOptions(o StudioOptions) (StudioOptions, error) {
	switch {
	case o.Blind: // CV Studio keeps a blind CV's own summary, never writes one
		o.Summary = "none"
	case o.Summary != "none" && o.Summary != "short" && o.Summary != "detailed":
		o.Summary = "normal"
	}
	if o.SummaryFocus != "technology" && o.SummaryFocus != "leadership" && o.SummaryFocus != "commercial" {
		o.SummaryFocus = "general"
	}
	o.SummaryDirection = strings.Join(strings.Fields(strings.ReplaceAll(o.SummaryDirection, "---", " ")), " ")
	if o.Summary == "none" {
		o.SummaryFocus, o.SummaryDirection = "general", ""
	}
	if payWords.MatchString(o.SummaryDirection) {
		return o, apperr.Validation(apperr.FieldErrors{{Field: "summaryDirection",
			Message: "Leave pay out of the direction: the summary never mentions the candidate's pay, and pay is never sent to the AI."}})
	}
	return o, nil
}

// payWords spots a direction about the candidate's pay or naming an amount of money.
var payWords = regexp.MustCompile(`(?i)\b(salary|salaries|remuneration|compensation|pay|paid|package|ctc|epf|kwsp|ringgit)\b|\b(rm|myr|sgd|usd|idr|php|thb|aud|eur|gbp)\s?\d|[$€£]\s?\d|\b\d+(\.\d+)?\s?k\b`)

// StudioCV is a formatting run and, once done, its result.
type StudioCV = store.StudioCv

// StartStudio queues formatting one of a candidate's CVs CV Studio's way.
func (s *Service) StartStudio(ctx context.Context, p authz.Principal, candidateID string, o StudioOptions) (StudioCV, error) {
	if o.Alignment != "justify" {
		o.Alignment = "left"
	}
	if !o.Blind {
		o.NeutralGender = false
	}
	o, err := summaryOptions(o)
	if err != nil {
		return StudioCV{}, err
	}
	var out StudioCV
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.deps.Candidates.ForCV(ctx, tx, p, candidateID); err != nil {
			return err // not found, or not theirs to see
		}
		src, url, err := s.deps.SourceCVs.SourceCV(ctx, tx, o.DocumentID)
		if err != nil {
			return err
		}
		if src.CandidateID != candidateID || src.Source == "formatted" || !formattable[src.ContentType] {
			return apperr.New(api.ErrorCodeSTUDIOCVSOURCE, "Choose a CV this candidate sent (PDF or Word), not one made by the app.")
		}
		access, on, err := s.deps.AI.Allowed(ctx, tx)
		if err != nil {
			return err
		}
		if !on {
			return apperr.New(api.ErrorCodeSTUDIOCVOFF,
				"CV Studio formatting needs the AI: set up an AI provider in Settings, or wait for next month's allowance.")
		}
		focus := api.StudioCvSummaryFocus(o.SummaryFocus)
		var direction nullable.Nullable[string]
		if o.SummaryDirection != "" {
			direction.Set(o.SummaryDirection)
		}
		q := store.New(tx)
		id, err := q.NewStudioCvID(ctx)
		if err != nil {
			return err
		}
		if out, err = q.InsertStudioCv(ctx, store.InsertStudioCvParams{
			ID: id, CandidateID: candidateID, SourceDocumentID: src.ID, Blind: o.Blind, NeutralGender: o.NeutralGender,
			CorrectLanguage: o.CorrectLanguage, Alignment: o.Alignment,
		}); err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindCvFormatStudio, api.CvFormatStudioJob{
			StudioCvId: id, DownloadUrl: url, ContentType: src.ContentType, FileName: src.FileName, SizeBytes: src.SizeBytes,
			Blind: o.Blind, NeutralGender: o.NeutralGender, CorrectLanguage: o.CorrectLanguage,
			Alignment: api.StudioCvAlignment(o.Alignment), Summary: api.StudioCvSummary(o.Summary), Ai: access,
			SummaryFocus: &focus, SummaryDirection: direction,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "cv.studio_started", EntityType: "candidate", EntityID: candidateID, Changes: map[string]any{
			"studioCvId": id, "sourceDocumentId": src.ID, "blind": o.Blind, "correctLanguage": o.CorrectLanguage, "summary": o.Summary,
			"summaryFocus": o.SummaryFocus, "summaryDirection": o.SummaryDirection != "", // whether one was given, not its words
		}})
	})
	return out, err
}

// ListStudio returns a candidate's latest studio CVs, newest first.
func (s *Service) ListStudio(ctx context.Context, p authz.Principal, candidateID string) ([]StudioCV, error) {
	var out []StudioCV
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.deps.Candidates.ForCV(ctx, tx, p, candidateID); err != nil {
			return err
		}
		var err error
		out, err = store.New(tx).ListStudioCvs(ctx, candidateID)
		return err
	})
	return out, err
}

// saveStudio handles cv.studio_formatted: the Word file, saved as the
// candidate's formatted CV, or why there is none.
func (s *Service) saveStudio(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.CvStudioFormattedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	if tokens := r.InputTokens + r.OutputTokens; tokens > 0 { // counted even when it failed
		if err := q.RecordStudioCvAIUse(ctx, store.RecordStudioCvAIUseParams{StudioCvID: r.StudioCvId, Tokens: int32(min(tokens, 1<<30))}); err != nil { //nolint:gosec // bounded above
			return err
		}
	}
	run, err := q.GetStudioCv(ctx, r.StudioCvId)
	if database.IsNoRows(err) || (err == nil && run.Status != "queued") {
		return nil // a repeated result
	}
	if err != nil {
		return err
	}
	fail := func(msg string) error {
		_, err := q.FinishStudioCv(ctx, store.FinishStudioCvParams{ID: run.ID, Status: "failed", Error: &msg, PayRemoved: int32(r.PayStatementsRemoved)}) //nolint:gosec // a small count
		return err
	}
	encoded, err := r.DocxBase64.Get()
	if err != nil || encoded == "" {
		msg := "It could not be formatted."
		if e, err := r.Error.Get(); err == nil && e != "" {
			msg = clip("It could not be formatted: "+e, 2000)
		}
		return fail(msg)
	}
	docx, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !strings.HasPrefix(string(docx), "PK") {
		return fail("The formatted file could not be read.")
	}
	p := authz.System(job.TenantID)
	cand, err := s.deps.Candidates.ForCV(ctx, tx, p, run.CandidateID)
	if e, ok := apperr.As(err); database.IsNoRows(err) || (ok && e.Code == api.ErrorCodeNOTFOUND) {
		return fail("The candidate is gone.")
	}
	if err != nil {
		return err
	}
	if run.Blind {
		leaks, err := Leaks(docx, IdentityOf(cand))
		if err != nil {
			return err
		}
		if len(leaks) > 0 { // never save it
			return fail("This blind CV would still show the candidate's " + strings.Join(leaks, " and ") + ", so it was not saved.")
		}
	}
	agency, err := s.deps.Tenants.TenantName(ctx, tx, job.TenantID)
	if err != nil {
		return err
	}
	name := FileName(cand, agency, run.Blind)
	docID, undo, err := s.deps.Documents.AddFormatted(ctx, tx, job.TenantID, run.CandidateID, name, docx)
	if err != nil {
		undo()
		return err
	}
	var warning *string
	if w, err := r.Warning.Get(); err == nil && w != "" {
		w = clip(w, 4000)
		warning = &w
	}
	if _, err := q.FinishStudioCv(ctx, store.FinishStudioCvParams{
		ID: run.ID, Status: "done", DocumentID: &docID, FileName: &name, Warning: warning, PayRemoved: int32(r.PayStatementsRemoved), //nolint:gosec // a small count
	}); err != nil {
		undo()
		return err
	}
	logx.From(ctx).Info("studio cv saved", "studio_cv_id", run.ID, "document_id", docID, "warning", warning != nil)
	return audit.Record(ctx, tx, audit.Entry{Action: "cv.formatted", EntityType: "candidate", EntityID: run.CandidateID, Changes: map[string]any{
		"documentId": docID, "studioCvId": run.ID, "blind": run.Blind,
	}})
}

// TokensSince is the AI tokens studio CVs used since a time (the company's
// monthly allowance), in the caller's transaction.
func (s *Service) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).StudioCvTokensSince(ctx, since)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
