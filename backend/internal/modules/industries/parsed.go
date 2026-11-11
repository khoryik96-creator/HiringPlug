package industries

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// EnrichParsed validates employer selection and classification on the Go side after grounding.
// It changes the review suggestion only. Candidate tags remain additive and version checked.
func (m *Module) EnrichParsed(ctx context.Context, tx pgx.Tx, p *api.ParsedProfile) error {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return err
	}
	jobs := []Employment{}
	for _, j := range p.Experience {
		jobs = append(jobs, Employment{Name: sourced(j.Company), Title: sourced(j.Title), Start: partial(j.StartDate), End: partial(j.EndDate), Current: j.Current})
	}
	selected, _ := SelectEmployer(jobs, time.Now())
	p.SelectedEmployerIndex = nil
	p.Industry = nullable.NewNullNullable[api.SourcedText]()
	if selected < 0 {
		return nil
	}
	p.SelectedEmployerIndex = &selected
	j := jobs[selected]
	verified, err := m.service.records.VerifiedEmployer(ctx, tx, j.Name)
	if err != nil {
		return err
	}
	category, _ := Recognize(c.Items, j, verified)
	if category != nil {
		label := root(*category, catalogueMap(c.Items)).Label
		quote := p.Experience[selected].Company.MustGet().Quote
		p.Industry = nullable.NewNullableWithValue(api.SourcedText{Value: label, Quote: quote})
	}
	return nil
}
func sourced(v nullable.Nullable[api.SourcedText]) string {
	if x, e := v.Get(); e == nil {
		return x.Value
	}
	return ""
}
func partial(v nullable.Nullable[string]) string {
	if x, e := v.Get(); e == nil {
		return x
	}
	return ""
}

// RootOptionsIn is a read-only compatibility view for the old flat Settings endpoint.
func (m *Module) RootOptionsIn(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return nil, err
	}
	c, err := catalogueIn(ctx, tx)
	out := map[string]bool{}
	for _, i := range c.Items {
		if i.ParentId == nil {
			out[i.Label] = i.Retired
		}
	}
	return out, err
}
