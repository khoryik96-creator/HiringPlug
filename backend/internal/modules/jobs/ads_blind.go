package jobs

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// BlindView is an ad's blind version as people see it.
type BlindView struct {
	Title       string
	Description *string
	Edited      bool // a person wrote it; changes to the ad do not remake it
	Outdated    bool // the ad changed after that person wrote it
}

// storedBlind is a person's own blind text, as stored.
type storedBlind struct {
	Edited             bool
	Title, Description *string
	SourceHash         *string
}

// BlindEdit is a person's change to an ad's blind version.
type BlindEdit struct {
	Title          *string // their own blind title
	DescriptionSet bool    // Description was sent (nil clears it)
	Description    *string
	Remake         bool // drop their own text: make it from the ad again
}

func pointers(ads []Ad) []*Ad {
	out := make([]*Ad, len(ads))
	for i := range ads {
		out[i] = &ads[i]
	}
	return out
}

// clientOf is what identifies a job's client in its ads.
func (s *Service) clientOf(ctx context.Context, tx pgx.Tx, jobID string) (Client, error) {
	job, err := getRow(ctx, store.New(tx), jobID)
	if err != nil {
		return Client{}, err
	}
	names, err := s.companies.Names(ctx, tx, []string{job.CompanyID})
	if err != nil {
		return Client{}, err
	}
	website, err := s.companies.Website(ctx, tx, job.CompanyID)
	if err != nil {
		return Client{}, err
	}
	c := Client{Name: names[job.CompanyID], Website: website}
	if job.ContactID != nil {
		info, err := s.contacts.Info(ctx, tx, []string{*job.ContactID})
		if err != nil {
			return Client{}, err
		}
		if ci, ok := info[*job.ContactID]; ok {
			c.Contacts = append([]string{ci.Name}, strings.Fields(ci.Name)...) // the full name first
		}
	}
	return c, nil
}

// withBlind fills in the ads' blind versions (each job's client read once).
func (s *Service) withBlind(ctx context.Context, tx pgx.Tx, ads ...*Ad) error {
	clients := map[string]Client{}
	for _, ad := range ads {
		c, ok := clients[ad.JobID]
		if !ok {
			var err error
			if c, err = s.clientOf(ctx, tx, ad.JobID); err != nil {
				return err
			}
			clients[ad.JobID] = c
		}
		if err := withClient(c, ad); err != nil {
			return err
		}
	}
	return nil
}

// withClient fills in one ad's blind version: a person's own text when
// there is one, else made from the ad.
func withClient(c Client, ad *Ad) error {
	title, text := Blind(ad.Details, c)
	ad.Blind = BlindView{Title: title, Description: text}
	if st := ad.stored; st.Edited {
		ad.Blind.Edited = true
		if st.Title != nil {
			ad.Blind.Title = *st.Title
		}
		ad.Blind.Description = st.Description
		ad.Blind.Outdated = st.SourceHash == nil || *st.SourceHash != SourceHash(ad.Details)
	}
	return nil
}

// applyBlindEdit works out the blind text to store after a change: remade
// on request, a person's own text kept (and checked not to name the
// client), or none (made from the ad when read).
func applyBlindEdit(current Ad, after AdDetails, e BlindEdit, c Client) (storedBlind, error) {
	if e.Remake {
		return storedBlind{}, nil
	}
	st := current.stored
	if e.Title == nil && !e.DescriptionSet {
		return st, nil
	}
	if !st.Edited { // start from what the person saw
		title, text := Blind(current.Details, c)
		st = storedBlind{Title: &title, Description: text}
	}
	var errs apperr.FieldErrors
	if e.Title != nil {
		t := strings.Join(strings.Fields(*e.Title), " ")
		if t == "" {
			errs.Add("blindTitle", "is required")
		}
		st.Title = &t
	}
	if e.DescriptionSet {
		st.Description = squeezedLinesOrNil(e.Description)
	}
	if st.Title != nil && NamesClient(*st.Title, c) {
		errs.Add("blindTitle", "still names the client")
	}
	if st.Description != nil && NamesClient(*st.Description, c) {
		errs.Add("blindDescription", "still names the client")
	}
	hash := SourceHash(after)
	st.Edited, st.SourceHash = true, &hash
	return st, errs.Err()
}

func squeezedLinesOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
