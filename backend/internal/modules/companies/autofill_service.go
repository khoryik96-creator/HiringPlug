package companies

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/robots"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

// Autofill is how reading the website went, and the suggestions waiting.
type Autofill struct {
	Status      string
	CheckedAt   *time.Time
	Error       *string
	Suggestions []PendingSuggestion
}

// PendingSuggestion is a suggestion with the company's value now.
type PendingSuggestion struct {
	ID, Field, Value, Source string
	Current                  *string
}

// current is a company's value for a suggestion's field.
func current(d Details, field string) *string {
	switch field {
	case "name":
		return &d.Name
	case "phone":
		return d.Phone
	case "address":
		return d.Address
	case "city":
		return d.City
	case "country":
		return d.Country
	case "registrationNumber":
		return d.RegistrationNumber
	case "size":
		return d.Size
	case "about":
		return d.About
	}
	return nil
}

// patchFor sets one field to a suggested value.
func patchFor(field, value string) Patch {
	v := apiconv.Change[*string]{Set: true, Value: &value}
	var p Patch
	switch field {
	case "name":
		p.Name = apiconv.Change[string]{Set: true, Value: value}
	case "phone":
		p.Phone = v
	case "address":
		p.Address = v
	case "city":
		p.City = v
	case "country":
		p.Country = v
	case "registrationNumber":
		p.RegistrationNumber = v
	case "size":
		p.Size = v
	case "about":
		p.About = v
	}
	return p
}

func (s *Service) autofillIn(ctx context.Context, q *store.Queries, id string) (Autofill, error) {
	row, err := q.GetCompany(ctx, id)
	if database.IsNoRows(err) {
		return Autofill{}, apperr.NotFound("company")
	}
	if err != nil {
		return Autofill{}, err
	}
	state, err := q.AutofillState(ctx, id)
	if err != nil {
		return Autofill{}, err
	}
	rows, err := q.PendingSuggestions(ctx, id)
	if err != nil {
		return Autofill{}, err
	}
	out := Autofill{Status: state.AutofillStatus, CheckedAt: state.AutofillCheckedAt, Error: state.AutofillError, Suggestions: []PendingSuggestion{}}
	details := fromRow(row).Details
	for _, r := range rows {
		out.Suggestions = append(out.Suggestions, PendingSuggestion{ID: r.ID, Field: r.Field, Value: r.Value, Source: r.Source, Current: current(details, r.Field)})
	}
	return out, nil
}

// Autofill returns the suggestions waiting, and how the last reading went.
func (s *Service) Autofill(ctx context.Context, p authz.Principal, id string) (Autofill, error) {
	var out Autofill
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.autofillIn(ctx, store.New(tx), id)
		return err
	})
	return out, err
}

// StartAutofill queues reading the company's website.
func (s *Service) StartAutofill(ctx context.Context, p authz.Principal, id string) (Autofill, error) {
	var out Autofill
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		state, err := q.AutofillState(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("company")
		}
		if err != nil {
			return err
		}
		if state.Website == nil {
			return apperr.Validation(apperr.FieldErrors{{Field: "website", Message: "add the company's website first"}})
		}
		if err := q.SetAutofill(ctx, store.SetAutofillParams{ID: id, AutofillStatus: "queued"}); err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindCompanyAutofill, api.CompanyAutofillJob{CompanyId: id}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "company.autofill_requested", EntityType: "company", EntityID: id}); err != nil {
			return err
		}
		out, err = s.autofillIn(ctx, q, id)
		return err
	})
	return out, err
}

// Decide accepts a suggestion (the company's detail is set, by the usual
// rules) or dismisses it.
func (s *Service) Decide(ctx context.Context, p authz.Principal, id, suggestionID string, accept bool) (Autofill, error) {
	var out Autofill
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		sug, err := q.GetSuggestion(ctx, store.GetSuggestionParams{ID: suggestionID, CompanyID: id})
		if database.IsNoRows(err) || (err == nil && sug.Status != "pending") {
			return apperr.NotFound("suggestion")
		}
		if err != nil {
			return err
		}
		status := "dismissed"
		if accept {
			status = "accepted"
			row, err := q.GetCompany(ctx, id)
			if err != nil {
				return err
			}
			if _, err := update(ctx, tx, q, row, patchFor(sug.Field, sug.Value), map[string]any{"from": sug.Source}); err != nil {
				return err
			}
		}
		if err := q.DecideSuggestion(ctx, store.DecideSuggestionParams{ID: sug.ID, Status: status}); err != nil {
			return err
		}
		out, err = s.autofillIn(ctx, q, id)
		return err
	})
	return out, err
}

const maxPage = 3 << 20

// autofill handles company.autofill: it reads the company's home page and
// its contact page (as robots.txt allows) and keeps the details that differ
// from what is recorded, as suggestions.
func (s *Service) autofill(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.CompanyAutofillJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.GetCompany(ctx, payload.CompanyId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	details := fromRow(row).Details
	if details.Website == nil {
		return q.SetAutofill(ctx, store.SetAutofillParams{ID: row.ID, AutofillStatus: "idle"})
	}
	found, readErr := s.readSite(ctx, *details.Website)
	if readErr != nil {
		msg := readErr.Error()
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
		return q.SetAutofill(ctx, store.SetAutofillParams{ID: row.ID, AutofillStatus: "failed", AutofillError: &msg})
	}
	if err := q.ClearSuggestions(ctx, row.ID); err != nil {
		return err
	}
	kept := 0
	for _, sug := range found {
		if now := current(details, sug.Field); now != nil && strings.EqualFold(strings.TrimSpace(*now), sug.Value) {
			continue // already recorded
		}
		if err := q.InsertSuggestion(ctx, store.InsertSuggestionParams{CompanyID: row.ID, Field: sug.Field, Value: sug.Value, Source: sug.Source}); err != nil {
			return err
		}
		kept++
	}
	if err := q.SetAutofill(ctx, store.SetAutofillParams{ID: row.ID, AutofillStatus: "done"}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "company.autofilled", EntityType: "company", EntityID: row.ID, Changes: map[string]any{"suggestions": kept}})
}

// readSite reads the home page, then the contact page for what is missing.
func (s *Service) readSite(ctx context.Context, website string) ([]Suggestion, error) {
	client := safehttp.Client(safehttp.Options{AllowPrivate: s.allowPrivate, Timeout: 15 * time.Second, FollowRedirect: true})
	site, err := robots.Read(ctx, client, website)
	if err != nil {
		return nil, fmt.Errorf("the website could not be reached: %w", err)
	}
	if !site.Allows(website) {
		return nil, errors.New("the website's robots.txt asks robots not to read it, so it was not read")
	}
	get := func(u string) ([]byte, error) {
		return safehttp.Get(ctx, client, u, robots.UserAgent, "text/html, */*;q=0.5", maxPage)
	}
	page, err := get(website)
	if err != nil {
		return nil, fmt.Errorf("the website could not be read: %w", err)
	}
	found := ReadSite(page, website)
	if contact := ContactPage(page, website); contact != "" && site.Allows(contact) {
		if more, err := get(contact); err == nil {
			found = Merge(found, ReadSite(more, contact))
		}
	}
	return found, nil
}
