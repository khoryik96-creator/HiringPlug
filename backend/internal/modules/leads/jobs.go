package leads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/robots"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

// userAgent names us to the sites we read (robots.txt rules for
// "HiringPlugBot" apply).
const userAgent = robots.UserAgent

// feeds are the public job board APIs, by kind (%s: the board name).
var feeds = map[string]string{
	Greenhouse:      "https://boards-api.greenhouse.io/v1/boards/%s/jobs",
	Lever:           "https://api.lever.co/v0/postings/%s?mode=json",
	Ashby:           "https://api.ashbyhq.com/posting-api/job-board/%s",
	SmartRecruiters: "https://api.smartrecruiters.com/v1/companies/%s/postings?limit=100",
}

const maxBody = 5 << 20

// check handles lead_source.check: it fetches a source's openings, updates
// its leads, records how it went, and queues the next check a day later. A
// job for a time the source is no longer scheduled for (replaced by "check
// now", or the source turned off) does nothing.
func (s *Service) check(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.LeadSourceCheckJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	src, err := q.GetSource(ctx, payload.SourceId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !src.Active || src.NextCheckAt == nil || !src.NextCheckAt.Equal(payload.RunAt) {
		return nil
	}
	postings, fetchErr := s.fetch(ctx, src)
	next := now().Add(CheckEvery)
	open, name := src.OpenLeads, src.Name
	changes := map[string]any{}
	if fetchErr == nil {
		sync, err := s.apply(ctx, q, src, postings)
		if err != nil {
			return err
		}
		open = int32(len(sync.New) + len(sync.Seen)) //nolint:gosec // a page of openings
		changes["new"], changes["closed"], changes["open"] = len(sync.New), len(sync.Closed), open
		if src.Kind == CareersPage && len(postings) > 0 && postings[0].Company != "" && name == defaultName(src.Identifier) {
			name = postings[0].Company // the markup names the company
		}
	} else {
		changes["error"] = fetchErr.Error()
	}
	var lastError *string
	if fetchErr != nil {
		msg := fetchErr.Error()
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
		lastError = &msg
	}
	if err := q.RecordCheck(ctx, store.RecordCheckParams{ID: src.ID, LastError: lastError, OpenLeads: open, NextCheckAt: &next, Name: name}); err != nil {
		return err
	}
	if err := queueCheck(ctx, tx, job.TenantID, src.ID, next); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "lead_source.checked", EntityType: "lead_source", EntityID: src.ID, Changes: changes})
}

func defaultName(identifier string) string {
	_, name, _ := NormalizeSource(CareersPage, identifier, "")
	return name
}

// apply writes what a check found: new leads, leads seen again, leads closed.
func (s *Service) apply(ctx context.Context, q *store.Queries, src store.LeadSource, postings []Posting) (Sync, error) {
	rows, err := q.SourceLeads(ctx, &src.ID)
	if err != nil {
		return Sync{}, err
	}
	existing := make([]Existing, 0, len(rows))
	for _, r := range rows {
		if r.ExternalID != nil {
			existing = append(existing, Existing{ID: r.ID, ExternalID: *r.ExternalID, Closed: r.ClosedAt != nil})
		}
	}
	sync := Diff(existing, postings)
	company := func(p Posting) string {
		if p.Company != "" {
			return p.Company
		}
		return src.Name
	}
	for _, p := range sync.New {
		externalID := p.ExternalID
		if _, err := q.InsertLead(ctx, store.InsertLeadParams{
			SourceID: &src.ID, SourceKind: &src.Kind, ExternalID: &externalID, CompanyName: company(p), CompanyID: src.CompanyID,
			Title: p.Title, Location: p.Location, Url: p.URL, PostedAt: p.PostedAt,
		}); err != nil {
			return Sync{}, err
		}
	}
	for id, p := range sync.Seen {
		if err := q.SeenLead(ctx, store.SeenLeadParams{ID: id, Title: p.Title, Location: p.Location, Url: p.URL, CompanyName: company(p)}); err != nil {
			return Sync{}, err
		}
	}
	for _, id := range sync.Closed {
		if err := q.CloseLead(ctx, id); err != nil {
			return Sync{}, err
		}
	}
	return sync, nil
}

// fetch reads a source's openings.
func (s *Service) fetch(ctx context.Context, src store.LeadSource) ([]Posting, error) {
	client := safehttp.Client(safehttp.Options{AllowPrivate: s.allowPrivate, Timeout: 20 * time.Second, FollowRedirect: true})
	if src.Kind != CareersPage {
		body, err := get(ctx, client, fmt.Sprintf(feeds[src.Kind], url.PathEscape(src.Identifier)))
		if err != nil {
			return nil, err
		}
		switch src.Kind {
		case Greenhouse:
			return ParseGreenhouse(body)
		case Lever:
			return ParseLever(body)
		case Ashby:
			return ParseAshby(body)
		default:
			return ParseSmartRecruiters(body, src.Identifier)
		}
	}
	site, err := robots.Read(ctx, client, src.Identifier)
	if err != nil {
		return nil, fmt.Errorf("reading robots.txt: %w", err)
	}
	if !site.Allows(src.Identifier) {
		return nil, errors.New("the site's robots.txt asks robots not to read this page, so it is not read")
	}
	body, err := get(ctx, client, src.Identifier)
	if err != nil {
		return nil, err
	}
	postings := ParseJobPostings(body, src.Identifier)
	if len(postings) == 0 {
		// Not "no openings": a page without markup cannot say, so nothing is closed.
		return nil, errors.New("no job markup (schema.org JobPosting) found on this page; ask the company for its job board address, or save its ads by hand")
	}
	return postings, nil
}

func get(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	return safehttp.Get(ctx, client, address, userAgent, "application/json, text/html;q=0.9, */*;q=0.5", maxBody)
}
