package app

// Adapters giving the intake module (CV imports) what it needs from the
// candidates and documents modules.

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries"
)

type intakeCandidates struct{ *candidates.Module }

func (a intakeCandidates) FillFromCV(ctx context.Context, tx pgx.Tx, id string, p api.ParsedProfile) (string, error) {
	return a.Module.FillFromCV(ctx, tx, id, cvFromParsed(p))
}

func (a intakeCandidates) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	briefs, err := a.Briefs(ctx, tx, ids)
	out := make(map[string]string, len(briefs))
	for id, b := range briefs {
		out[id] = b.Name
	}
	return out, err
}

type intakeDocuments struct{ *documents.Module }

func (intakeDocuments) IsCVFile(fileName string, data []byte) bool {
	return documents.Readable(fileName, data)
}

// value is a read value's words, or nil.
func value(n nullable.Nullable[api.SourcedText]) *string {
	s, err := n.Get()
	if err != nil || strings.TrimSpace(s.Value) == "" {
		return nil
	}
	v := strings.TrimSpace(s.Value)
	return &v
}

func plain[T any](n nullable.Nullable[T]) *T {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &v
}

// cvFromParsed turns a read CV into the candidate fields it fills: the name
// (first word, then the rest), contact details, places, LinkedIn, industry,
// notice period ("1 month"), skills, and the latest job as the current one.
func cvFromParsed(p api.ParsedProfile) candidates.FromCV {
	var cv candidates.FromCV
	if name := value(p.FullName); name != nil {
		first, last, _ := strings.Cut(*name, " ")
		cv.FirstName, cv.LastName = first, strings.TrimSpace(last)
	}
	cv.Profile = candidates.Profile{
		Email: value(p.Email), Phone: value(p.Phone), Location: value(p.Location), Nationality: value(p.Nationality),
		ResidenceCountry: value(p.ResidenceCountry), LinkedinURL: value(p.LinkedinUrl), Industry: value(p.Industry),
	}
	if n := value(p.NoticePeriod); n != nil {
		if num, unit, ok := strings.Cut(*n, " "); ok {
			if v, err := strconv.Atoi(num); err == nil {
				cv.Profile.NoticePeriod = &candidates.NoticePeriod{Value: v, Unit: unit}
			}
		}
	}
	for _, s := range p.Skills {
		cv.Profile.Skills = append(cv.Profile.Skills, s.Value)
	}
	jobs := []industries.Employment{}
	for _, j := range p.Experience {
		job := candidates.Job{Title: value(j.Title), Company: value(j.Company), StartDate: plain(j.StartDate), EndDate: plain(j.EndDate), Current: j.Current}
		cv.History.Work = append(cv.History.Work, job)
		jobs = append(jobs, industries.Employment{Name: plainString(job.Company), Title: plainString(job.Title), Start: plainString(job.StartDate), End: plainString(job.EndDate), Current: job.Current})
	}
	latest, _ := industries.SelectEmployer(jobs, time.Now())
	if latest >= 0 {
		cv.Profile.CurrentTitle, cv.Profile.CurrentCompany = cv.History.Work[latest].Title, cv.History.Work[latest].Company
	}
	for _, e := range p.Education {
		cv.History.Education = append(cv.History.Education, candidates.Qualification{
			Qualification: value(e.Qualification), Institution: value(e.Institution), EndYear: plain(e.EndYear),
		})
	}
	return cv
}

// documentCandidates gives the documents module whether a candidate exists
// and their names (whose a CV already is).
type documentCandidates struct{ *candidates.Module }

func (a documentCandidates) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	return intakeCandidates(a).Names(ctx, tx, ids)
}
