package industries

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Employment is selected-employer evidence only, never an undifferentiated CV.
type Employment struct {
	Name, Title, Start, End string
	Current, Primary        bool
}

// Evidence is read from the candidate record by an app adapter, not asserted by the browser.
type Evidence struct {
	Version   int
	Employers []Employment
}

var enterpriseRole = regexp.MustCompile(`(?i)\b(sap|s/?4\s*hana|abap|amdp|bpc|erp|acumatica|crm|hcm|application support|enterprise systems? support)\b`)

// DateBound handles ISO dates and partial dates without locale guesses.
func DateBound(value string, end bool) (time.Time, bool) {
	layout := "2006-01-02"
	if len(value) == 4 {
		layout = "2006"
	} else if len(value) == 7 {
		layout = "2006-01"
	}
	t, e := time.Parse(layout, value)
	if e != nil {
		return time.Time{}, false
	}
	if end {
		if len(value) == 4 {
			t = t.AddDate(1, 0, 0).Add(-time.Nanosecond)
		} else if len(value) == 7 {
			t = t.AddDate(0, 1, 0).Add(-time.Nanosecond)
		}
	}
	return t, true
}

// SelectEmployer prefers an unambiguous current employer, otherwise latest ended employment.
// An unrelated undated historical job cannot invalidate a clear current job.
func SelectEmployer(jobs []Employment, now time.Time) (int, string) {
	current := []int{}
	ended := []int{}
	bounds := map[int]time.Time{}
	uncertain := false
	incomplete := false
	for i, j := range jobs {
		if strings.TrimSpace(j.Name) == "" {
			continue
		}
		start, startOK := DateBound(j.Start, false)
		end, endOK := DateBound(j.End, true)
		if j.Start != "" && !startOK || j.End != "" && !endOK || startOK && endOK && start.After(end) {
			if j.Current {
				uncertain = true
			}
			incomplete = true
			continue
		}
		if startOK && start.After(now) {
			continue
		}
		if j.Current {
			if endOK && end.Before(now) {
				uncertain = true
				continue
			}
			current = append(current, i)
		} else if endOK && !end.After(now) {
			ended = append(ended, i)
			bounds[i] = end
		} else {
			incomplete = true
		}
	}
	if len(current) == 1 && !uncertain {
		return current[0], "Current employer"
	}
	if len(current) > 0 {
		primary := -1
		for _, i := range current {
			if jobs[i].Primary {
				if primary >= 0 {
					return -1, "Choose an employer; more than one is current."
				}
				primary = i
			}
		}
		if primary >= 0 && !uncertain {
			return primary, "Primary current employer"
		}
		return -1, "Choose an employer; more than one is current."
	}
	if uncertain || incomplete {
		return -1, "Review the conflicting employment dates, or choose an employer."
	}
	latest := -1
	tied := false
	for _, i := range ended {
		if latest < 0 || bounds[i].After(bounds[latest]) {
			latest = i
			tied = false
		} else if bounds[i].Equal(bounds[latest]) {
			tied = true
		}
	}
	if latest < 0 || tied {
		return -1, "Choose an employer to classify; the dates do not identify one latest employer."
	}
	return latest, "Latest ended employer"
}
func (s *Service) Suggest(ctx context.Context, p authz.Principal, id string, chosen *int) (api.IndustrySuggestion, error) {
	var out api.IndustrySuggestion
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.ensureIn(ctx, tx); err != nil {
			return err
		}
		e, err := s.records.EmployerEvidence(ctx, tx, id)
		if err != nil {
			return err
		}
		c, err := catalogueIn(ctx, tx)
		if err != nil {
			return err
		}
		out = api.IndustrySuggestion{CatalogueVersion: c.Version, CandidateVersion: e.Version, Status: api.IndustrySuggestionStatus("needs_review"), Employers: []api.IndustryEmployer{}}
		for i, j := range e.Employers {
			out.Employers = append(out.Employers, api.IndustryEmployer{Index: i, Name: j.Name, Title: j.Title, Current: j.Current})
		}
		selected, reason := SelectEmployer(e.Employers, time.Now())
		out.Reason = reason
		if chosen != nil {
			if *chosen < 0 || *chosen >= len(e.Employers) {
				return invalid("employerIndex", "choose an employer from this candidate's work history")
			}
			selected = *chosen
		}
		if selected < 0 {
			return nil
		}
		out.EmployerIndex = &selected
		j := e.Employers[selected]
		verified, err := s.records.VerifiedEmployer(ctx, tx, j.Name)
		if err != nil {
			return err
		}
		category, rule := Recognize(c.Items, j, verified)
		out.Reason = rule
		if category != nil {
			out.CategoryId = &category.Id
			out.Status = api.IndustrySuggestionStatus("ready")
		}
		return nil
	})
	return out, err
}

// Recognize preserves a verified specific sector before considering role fallbacks.
func Recognize(items []api.Industry, j Employment, verifiedID string) (*api.Industry, string) {
	all := catalogueMap(items)
	if i, ok := all[verifiedID]; ok && active(i, all) {
		if i.ParentId != nil {
			return &i, "Verified employer subindustry"
		}
		if originalLabel(i) == "Information Technology & Services" && enterpriseRole.MatchString(j.Title) {
			if erp := semantic(items, "Tech – ERP/CRM/HCM/Application"); erp != nil && active(*erp, all) && erp.ParentId != nil && *erp.ParentId == i.Id {
				return erp, "Enterprise applications role at a verified IT employer"
			}
		}
		return &i, "Verified employer industry"
	}
	// Employer-name rules are narrow, bounded matches. Job-title words alone never establish a sector.
	for _, rule := range employerRules {
		if rule.pattern.MatchString(j.Name) {
			if item := semantic(items, rule.label); item != nil && active(*item, all) {
				return item, "Employer business recognised from its name"
			}
		}
	}
	return nil, "Employer industry is unclear. Choose it manually; saving remains available."
}
func originalLabel(i api.Industry) string {
	if i.SourceKey == nil {
		return ""
	}
	for _, g := range seedGroups() {
		if g.ID == *i.SourceKey {
			return g.Label
		}
	}
	return ""
}
func semantic(items []api.Industry, label string) *api.Industry {
	source := ""
	for _, g := range seedGroups() {
		if g.Label == label {
			source = g.ID
		}
		for _, c := range g.Children {
			if c.Label == label {
				source = c.ID
			}
		}
	}
	if source == "" {
		return nil
	}
	for _, i := range items {
		if i.SourceKey != nil && *i.SourceKey == source {
			return &i
		}
	}
	return nil
}
