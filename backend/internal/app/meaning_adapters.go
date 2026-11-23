package app

// What search by meaning (the meaning module) reads of candidates and CVs,
// through the candidates and documents modules.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

type meaningCandidates struct{ candidates *candidates.Module }

func meaningSources(rows []candidates.MeaningSource) []meaning.Source {
	out := make([]meaning.Source, len(rows))
	for i, r := range rows {
		out[i] = meaning.Source{CandidateID: r.ID, CurrentTitle: r.CurrentTitle}
		for _, w := range r.Work {
			out[i].Work = append(out[i].Work, meaning.Work{
				Position: w.Position, Title: w.Title, Description: w.Detail, StartDate: w.StartDate, EndDate: w.EndDate, Current: w.Current,
			})
		}
	}
	return out
}

func (a meaningCandidates) SourcesAfter(ctx context.Context, tx pgx.Tx, cursorID string, limit int) ([]meaning.Source, error) {
	rows, err := a.candidates.MeaningSourcesAfter(ctx, tx, cursorID, limit)
	return meaningSources(rows), err
}

func (a meaningCandidates) Sources(ctx context.Context, tx pgx.Tx, ids []string) ([]meaning.Source, error) {
	rows, err := a.candidates.MeaningSources(ctx, tx, ids)
	return meaningSources(rows), err
}

func (a meaningCandidates) Count(ctx context.Context, tx pgx.Tx) (int, error) {
	return a.candidates.Count(ctx, tx)
}

type meaningCVs struct{ documents *documents.Module }

func (a meaningCVs) NewestCVs(ctx context.Context, tx pgx.Tx, candidateIDs []string) (map[string]meaning.CV, error) {
	found, err := a.documents.NewestCVTexts(ctx, tx, candidateIDs)
	out := make(map[string]meaning.CV, len(found))
	for id, cv := range found {
		out[id] = meaning.CV{DocumentID: cv.DocumentID, Text: cv.Text}
	}
	return out, err
}

func (a meaningCVs) CandidateOf(ctx context.Context, tx pgx.Tx, documentID string) (string, error) {
	candidateID, source, _, err := a.documents.CVTextOf(ctx, tx, documentID)
	if database.IsNoRows(err) || source == "formatted" {
		return "", nil
	}
	return candidateID, err
}

func (a meaningCVs) SearchCVs(ctx context.Context, tx pgx.Tx, terms []meaning.Term, exclude []string, limit int) ([]meaning.CVHit, error) {
	words := make([]documents.SearchTerm, len(terms))
	for i, t := range terms {
		words[i] = documents.SearchTerm{Text: t.Text, Weight: t.Weight}
	}
	hits, err := a.documents.SearchCVs(ctx, tx, words, exclude, limit)
	out := make([]meaning.CVHit, len(hits))
	for i, h := range hits {
		out[i] = meaning.CVHit{CandidateID: h.CandidateID, Rank: h.Rank, Snippet: h.Snippet}
	}
	return out, err
}

func (a meaningCVs) DocumentExists(ctx context.Context, tx pgx.Tx, documentID string) (bool, error) {
	return a.documents.DocumentExists(ctx, tx, documentID)
}

// candidatesMeaning lets the advanced search rank by meaning.
type candidatesMeaning struct{ meaning *meaning.Module }

func (a candidatesMeaning) Search(ctx context.Context, tx pgx.Tx, p authz.Principal, text string, limit int) (api.MeaningOutcome, []candidates.MeaningHit, error) {
	outcome, ranked, err := a.meaning.Rank(ctx, tx, p, meaning.Ask{Body: text, Limit: limit})
	hits := make([]candidates.MeaningHit, len(ranked))
	for i, r := range ranked {
		hits[i] = candidates.MeaningHit{CandidateID: r.CandidateID, Why: r.Why}
	}
	return outcome, hits, err
}

// matchingMeaning finds a job's possible candidates by meaning.
type matchingMeaning struct{ meaning *meaning.Module }

func (a matchingMeaning) ForJob(ctx context.Context, tx pgx.Tx, p authz.Principal, title, text string, exclude []string,
	words []matching.CVHit, limit int,
) (api.MeaningOutcome, []matching.MeaningMatch, error) {
	if strings.TrimSpace(title) == "" { // read as a job, never as a typed search
		title, _, _ = strings.Cut(strings.TrimSpace(text), "\n")
	}
	hits := make([]meaning.CVHit, len(words))
	for i, w := range words {
		hits[i] = meaning.CVHit{CandidateID: w.CandidateID, Rank: w.Rank, Snippet: w.Snippet}
	}
	outcome, ranked, err := a.meaning.Rank(ctx, tx, p, meaning.Ask{Title: title, Body: text, Exclude: exclude, Words: hits, Limit: limit})
	out := make([]matching.MeaningMatch, len(ranked))
	for i, r := range ranked {
		out[i] = matching.MeaningMatch{CandidateID: r.CandidateID, Score: r.Score, Why: r.Why}
	}
	return outcome, out, err
}
