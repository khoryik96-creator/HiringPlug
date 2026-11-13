package candidates

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// UsedValues are tags, skills, industries and specializations already used in
// the company, most used first.
type UsedValues struct {
	Tags, Skills, Industries, Specializations []string
}

// usedFor is how long the suggestions are remembered. Counting them reads
// every candidate (seconds with hundreds of thousands), and they are only
// suggestions: a value not seen before forgets them at once (learn), so a
// new tag shows straight away; only their order may be a few minutes old.
const usedFor = 5 * time.Minute

type usedEntry struct {
	at     time.Time
	values UsedValues
	known  map[string]bool
}

// usedCache remembers the suggestions per company and per what the person
// may see (a role limited to its own candidates gets its own).
type usedCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]map[string]usedEntry // company -> who sees what -> suggestions
}

func (c *usedCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *usedCache) get(tenantID, scope string) (UsedValues, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tenantID][scope]
	if !ok || c.clock().Sub(e.at) > usedFor {
		return UsedValues{}, false
	}
	return e.values, true
}

func usedKey(kind, value string) string { return kind + "\x00" + strings.ToLower(value) }

func (c *usedCache) put(tenantID, scope string, v UsedValues) {
	known := map[string]bool{}
	for kind, values := range map[string][]string{"tag": v.Tags, "skill": v.Skills, "industry": v.Industries, "specialization": v.Specializations} {
		for _, value := range values {
			known[usedKey(kind, value)] = true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]map[string]usedEntry{}
	}
	if c.entries[tenantID] == nil {
		c.entries[tenantID] = map[string]usedEntry{}
	}
	c.entries[tenantID][scope] = usedEntry{at: c.clock(), values: v, known: known}
}

// learn is told the labels of a profile just saved: suggestions that
// do not have one of them yet are forgotten, so it is suggested from now on.
func (c *usedCache) learn(tenantID string, p Profile) {
	labels := make([]string, 0, len(p.Tags)+len(p.Skills)+2)
	for _, t := range p.Tags {
		labels = append(labels, usedKey("tag", t))
	}
	for _, s := range p.Skills {
		labels = append(labels, usedKey("skill", s))
	}
	if p.Industry != nil {
		labels = append(labels, usedKey("industry", *p.Industry))
	}
	if p.Specialization != nil {
		labels = append(labels, usedKey("specialization", *p.Specialization))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for scope, e := range c.entries[tenantID] {
		for _, l := range labels {
			if !e.known[l] {
				delete(c.entries[tenantID], scope) // counted again for those who see this much
				break
			}
		}
	}
}

// Used returns the values already used, for suggestions.
func (s *Service) Used(ctx context.Context, p authz.Principal) (UsedValues, error) {
	scope := p.ScopeUsersSetting() + "|" + p.ScopeJobsSetting()
	if v, ok := s.used.get(p.TenantID, scope); ok {
		return v, nil
	}
	out := UsedValues{Tags: []string{}, Skills: []string{}, Industries: []string{}, Specializations: []string{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).UsedValues(ctx)
		for _, r := range rows {
			switch r.Kind {
			case "tag":
				out.Tags = append(out.Tags, r.Value)
			case "skill":
				out.Skills = append(out.Skills, r.Value)
			case "industry":
				out.Industries = append(out.Industries, r.Value)
			case "specialization":
				out.Specializations = append(out.Specializations, r.Value)
			}
		}
		return err
	})
	if err == nil {
		s.used.put(p.TenantID, scope, out)
	}
	return out, err
}
