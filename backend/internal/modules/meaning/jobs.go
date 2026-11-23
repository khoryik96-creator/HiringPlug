package meaning

// Saving what the AI worker read: candidates (meaning.candidates_read) and
// searches or jobs (meaning.query_read). The vectors go into the database
// as text and are compared there; nothing here reads them back.

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// vector writes a vector as pgvector reads it: [0.1,0.2,...].
func vector(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 9)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func optionalVector(v *api.MeaningVector) *string {
	if v == nil {
		return nil
	}
	s := vector(*v)
	return &s
}

func smallint(n *int) int16 {
	if n == nil {
		return -1
	}
	return int16(*n) //nolint:gosec // a level or a year
}

// candidatesRead handles meaning.candidates_read.
func (s *Service) candidatesRead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.MeaningCandidatesReadJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	if payload.Unavailable != nil && *payload.Unavailable {
		logx.From(ctx).Warn("search by meaning: the AI worker has no meaning model (task setup downloads it)")
		until := s.now().Add(unavailable)
		return q.SetUnavailable(ctx, &until)
	}
	if err := q.SetSweepModel(ctx, &payload.Model); err != nil {
		return err
	}
	ids := make([]string, len(payload.Candidates))
	for i, c := range payload.Candidates {
		ids[i] = c.CandidateId
	}
	sources, err := s.deps.Candidates.Sources(ctx, tx, ids) // those still here
	if err != nil {
		return err
	}
	here := map[string]bool{}
	for _, src := range sources {
		here[src.CandidateID] = true
	}
	for _, c := range payload.Candidates {
		if !here[c.CandidateId] {
			continue // deleted while being read
		}
		if err := s.saveRead(ctx, tx, payload.Model, c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) saveRead(ctx context.Context, tx pgx.Tx, model string, c api.MeaningCandidateRead) error {
	q := store.New(tx)
	if err := q.DeleteRoles(ctx, c.CandidateId); err != nil {
		return err
	}
	if err := q.DeletePassages(ctx, c.CandidateId); err != nil {
		return err
	}
	if err := q.SaveRead(ctx, store.SaveReadParams{
		CandidateID: c.CandidateId, Fingerprint: c.Fingerprint, Model: model, Profile: optionalVector(c.Profile),
	}); err != nil {
		return err
	}
	if len(c.Roles) > 0 {
		in := store.InsertRolesParams{CandidateID: c.CandidateId}
		for _, r := range c.Roles {
			family := ""
			if r.Family != nil {
				family = *r.Family
			}
			in.Ordinals = append(in.Ordinals, int16(r.Ordinal)) //nolint:gosec // at most 30 (contract)
			in.Titles = append(in.Titles, r.Title)
			in.Families = append(in.Families, family)
			in.Levels = append(in.Levels, smallint(r.Level))
			var end *int
			if y, err := r.EndYear.Get(); err == nil {
				end = &y
			}
			in.EndYears = append(in.EndYears, smallint(end))
			in.Fields = append(in.Fields, vector(r.Field))
			in.Works = append(in.Works, vector(r.Work))
		}
		if err := q.InsertRoles(ctx, in); err != nil {
			return err
		}
	}
	if len(c.Passages) > 0 {
		// Only of a CV that is still there (it may have been deleted while being read).
		exists, err := s.deps.CVs.DocumentExists(ctx, tx, c.Passages[0].DocumentId)
		if err != nil {
			return err
		}
		if exists {
			in := store.InsertPassagesParams{CandidateID: c.CandidateId}
			for _, p := range c.Passages {
				in.Ordinals = append(in.Ordinals, int16(p.Ordinal)) //nolint:gosec // at most 40 (contract)
				in.DocumentIds = append(in.DocumentIds, p.DocumentId)
				in.Passages = append(in.Passages, p.Passage)
				in.Vecs = append(in.Vecs, vector(p.Vec))
			}
			if err := q.InsertPassages(ctx, in); err != nil {
				return err
			}
		}
	}
	return q.ClearPending(ctx, store.ClearPendingParams{CandidateID: c.CandidateId, Fingerprint: c.Fingerprint})
}

// queryRead handles meaning.query_read.
func (s *Service) queryRead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.MeaningQueryReadJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	if payload.Unavailable != nil && *payload.Unavailable {
		until := s.now().Add(unavailable)
		if err := q.SetUnavailable(ctx, &until); err != nil {
			return err
		}
		return q.FailQuery(ctx, payload.QueryId)
	}
	understood := []byte("null")
	if payload.Understood != nil {
		var err error
		if understood, err = json.Marshal(payload.Understood); err != nil {
			return err
		}
	}
	var level *int16
	if payload.Level != nil {
		l := smallint(payload.Level)
		level = &l
	}
	n, err := q.SaveQuery(ctx, store.SaveQueryParams{
		ID: payload.QueryId, Role: optionalVector(payload.Role), Whole: optionalVector(payload.Whole),
		Context: optionalVector(payload.Context), Level: level, Describes: payload.Describes,
		Exact: payload.Exact != nil && *payload.Exact, Understood: understood,
	})
	if err != nil || n == 0 {
		return err // n == 0: the search was forgotten meanwhile
	}
	if err := q.DeleteNeeds(ctx, payload.QueryId); err != nil {
		return err
	}
	if len(payload.Needs) > 0 {
		in := store.InsertNeedsParams{QueryID: payload.QueryId}
		for i, need := range payload.Needs {
			in.Ordinals = append(in.Ordinals, int16(i)) //nolint:gosec // at most 12 (contract)
			in.Needs = append(in.Needs, need.Need)
			in.Vecs = append(in.Vecs, vector(need.Vec))
		}
		if err := q.InsertNeeds(ctx, in); err != nil {
			return err
		}
	}
	return q.SetSweepModel(ctx, &payload.Model)
}
