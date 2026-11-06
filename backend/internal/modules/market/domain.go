package market

// Pure rules: which words of a job title to look for, what the pool of
// candidates looks like, and the map laid out as a document.

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/country"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

// Level and filler words left out of a title: "Senior Finance Manager" is
// mapped as "finance", so finance people at every level are counted.
var levelWords = map[string]bool{
	"senior": true, "sr": true, "junior": true, "jr": true, "lead": true, "head": true, "principal": true, "chief": true,
	"associate": true, "assistant": true, "deputy": true, "vice": true, "president": true, "vp": true, "director": true,
	"manager": true, "executive": true, "officer": true, "intern": true, "trainee": true, "staff": true, "general": true,
	"of": true, "and": true, "the": true, "for": true, "in": true, "i": true, "ii": true, "iii": true,
}

// TitleWords are the words of a job title a candidate's title must have,
// lower case: the title less its level words (all of it when nothing is left).
func TitleWords(title string) string {
	all := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#' })
	var kept []string
	for _, w := range all {
		if !levelWords[w] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		kept = all
	}
	return strings.Join(kept, " ")
}

// Member is one candidate counted (no names or contact details).
type Member struct {
	Company, Location, Country, Availability, WorkAuth, WorkBasis string
	NoticeValue                                                   *int
	NoticeUnit                                                    string
}

// Pay is a member's pay ("" when not recorded).
type Pay struct {
	CurrentAmount, CurrentCurrency, CurrentPeriod    string
	ExpectedAmount, ExpectedCurrency, ExpectedPeriod string
}

// Count is a value and how many have it.
type Count struct {
	Key   string
	Count int
}

// Range is the spread of amounts: the 25th percentile, the median and the 75th.
type Range struct {
	Count             int
	Low, Median, High float64
}

// PayGroup is pay in one currency and period.
type PayGroup struct {
	Currency, Period  string
	Current, Expected *Range
}

// Pool is who in the database does the work.
type Pool struct {
	Total             int
	Availability      []Count
	WorkAuthorization []Count
	Companies         []Count
	Locations         []Count
	MedianNoticeDays  *int
	Salary            []PayGroup
}

// Build counts the members and spreads their pay.
func Build(members []Member, pays []Pay) Pool {
	pool := Pool{Total: len(members)}
	avail, auth, companies, places := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var notice []float64
	for _, m := range members {
		avail[or(m.Availability, "unknown")]++
		auth[authKind(m.WorkAuth, m.WorkBasis)]++
		if c := strings.TrimSpace(m.Company); c != "" {
			companies[c]++
		}
		if place := placeOf(m); place != "" {
			places[place]++
		}
		if d, ok := noticeDays(m.NoticeValue, m.NoticeUnit); ok {
			notice = append(notice, float64(d))
		}
	}
	pool.Availability = ordered(avail, 0)
	pool.WorkAuthorization = ordered(auth, 0)
	pool.Companies = ordered(companies, 10)
	pool.Locations = ordered(places, 8)
	if len(notice) > 0 {
		d := int(math.Round(percentile(notice, 0.5)))
		pool.MedianNoticeDays = &d
	}
	pool.Salary = payGroups(pays)
	return pool
}

func or(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// authKind is the candidate page's choice: local, pr, expat, needs_pass or unknown.
func authKind(status, basis string) string {
	switch status {
	case "local":
		return "local"
	case "requires_visa":
		return "needs_pass"
	case "expat_eligible":
		if basis == "permanent_resident" {
			return "pr"
		}
		return "expat"
	}
	return "unknown"
}

func placeOf(m Member) string {
	name := ""
	if m.Country != "" {
		name = country.Name(m.Country)
	}
	loc := strings.TrimSpace(m.Location)
	switch {
	case loc == "":
		return name
	case name == "" || strings.Contains(strings.ToLower(loc), strings.ToLower(name)):
		return loc
	}
	return loc + ", " + name
}

func noticeDays(v *int, unit string) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch unit {
	case "day":
		return *v, true
	case "week":
		return *v * 7, true
	case "month":
		return *v * 30, true
	}
	return 0, false
}

// ordered is the counts, the most first (then by name), at most limit (0: all).
func ordered(m map[string]int, limit int) []Count {
	out := make([]Count, 0, len(m))
	for k, n := range m {
		out = append(out, Count{Key: k, Count: n})
	}
	slices.SortFunc(out, func(a, b Count) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(a.Key, b.Key)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func payGroups(pays []Pay) []PayGroup {
	type key struct{ currency, period string }
	current, expected := map[key][]float64{}, map[key][]float64{}
	add := func(m map[key][]float64, amount, currency, period string) {
		v, err := strconv.ParseFloat(amount, 64)
		if err != nil || v <= 0 || currency == "" || period == "" {
			return
		}
		k := key{currency, period}
		m[k] = append(m[k], v)
	}
	for _, p := range pays {
		add(current, p.CurrentAmount, p.CurrentCurrency, p.CurrentPeriod)
		add(expected, p.ExpectedAmount, p.ExpectedCurrency, p.ExpectedPeriod)
	}
	keys := map[key]int{}
	for k, v := range current {
		keys[k] += len(v)
	}
	for k, v := range expected {
		keys[k] += len(v)
	}
	out := make([]PayGroup, 0, len(keys))
	for k := range keys {
		out = append(out, PayGroup{Currency: k.currency, Period: k.period, Current: spread(current[k]), Expected: spread(expected[k])})
	}
	slices.SortFunc(out, func(a, b PayGroup) int {
		if c := cmp.Compare(keys[key{b.Currency, b.Period}], keys[key{a.Currency, a.Period}]); c != 0 {
			return c
		}
		return cmp.Compare(a.Currency+a.Period, b.Currency+b.Period)
	})
	return out
}

func spread(v []float64) *Range {
	if len(v) == 0 {
		return nil
	}
	return &Range{Count: len(v), Low: percentile(v, 0.25), Median: percentile(v, 0.5), High: percentile(v, 0.75)}
}

// percentile of values (interpolated between the nearest two).
func percentile(values []float64, q float64) float64 {
	v := slices.Clone(values)
	slices.Sort(v)
	pos := q * float64(len(v)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return v[lo] + (v[hi]-v[lo])*(pos-float64(lo))
}

// Amount is an amount as a whole number string, for the API.
func Amount(v float64) string { return strconv.FormatFloat(math.Round(v), 'f', 0, 64) }

// Money is an amount as people read it: "RM 12,000", "SGD 8,500".
func Money(currency string, v float64) string {
	digits := Amount(v)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 && r != '-' && digits[i-1] != '-' {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if currency == "MYR" {
		currency = "RM"
	}
	return currency + " " + b.String()
}

// Labels the map uses for statuses and work authorization.
var (
	AvailabilityLabel = map[string]string{"looking": "Looking out", "passive": "Passive", "placed": "Placed", "not_looking": "Not looking", "unknown": "Not set"}
	AuthLabel         = map[string]string{"local": "Local", "pr": "PR", "expat": "Spousal / RP-T", "needs_pass": "Needs pass", "unknown": "Not known"}
)

func counts(cs []Count, label map[string]string) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		k := c.Key
		if l, ok := label[k]; ok {
			k = l
		}
		parts[i] = fmt.Sprintf("%s %d", k, c.Count)
	}
	return strings.Join(parts, ", ")
}

// PoolText is the pool in a few lines for the AI: counts only, never pay.
func PoolText(searched string, p Pool) string {
	if p.Total == 0 {
		return fmt.Sprintf("No candidates in our database have %q in their current title.", searched)
	}
	lines := []string{
		fmt.Sprintf("%d candidates have %q in their current title.", p.Total, searched),
		"Status: " + counts(p.Availability, AvailabilityLabel),
		"Work authorization: " + counts(p.WorkAuthorization, AuthLabel),
	}
	if len(p.Companies) > 0 {
		lines = append(lines, "Where they work now: "+counts(p.Companies, nil))
	}
	if len(p.Locations) > 0 {
		lines = append(lines, "Where they live: "+counts(p.Locations, nil))
	}
	if p.MedianNoticeDays != nil {
		lines = append(lines, fmt.Sprintf("Median notice: %d days", *p.MedianNoticeDays))
	}
	return strings.Join(lines, "\n")
}

// Insight is the AI's market insights, when asked for and answered.
type Insight struct {
	Text, Model string
	AskedAt     time.Time
}

// Doc lays the map out for Word or PDF.
func Doc(jobTitle, searched string, p Pool, salaryRestricted bool, insight *Insight, generated time.Time) simpledoc.Doc {
	doc := simpledoc.Doc{
		{Kind: simpledoc.Title, Text: "Market map: " + jobTitle},
		{Kind: simpledoc.Muted, Text: fmt.Sprintf("Candidates in our database with %q in their current title · %s", searched, generated.Format("2 Jan 2006"))},
		{Kind: simpledoc.Heading, Text: "In our database"},
	}
	if p.Total == 0 {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Paragraph, Text: "No candidates yet."})
	} else {
		doc = append(doc,
			simpledoc.Block{Kind: simpledoc.Bullet, Text: fmt.Sprintf("Candidates: %d", p.Total)},
			simpledoc.Block{Kind: simpledoc.Bullet, Text: "Status: " + counts(p.Availability, AvailabilityLabel)},
			simpledoc.Block{Kind: simpledoc.Bullet, Text: "Work authorization: " + counts(p.WorkAuthorization, AuthLabel)},
		)
		if p.MedianNoticeDays != nil {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: fmt.Sprintf("Median notice: %d days", *p.MedianNoticeDays)})
		}
		if len(p.Companies) > 0 {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Where they work now"})
			for _, c := range p.Companies {
				doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: fmt.Sprintf("%s (%d)", c.Key, c.Count)})
			}
		}
		if len(p.Locations) > 0 {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Where they live"})
			for _, c := range p.Locations {
				doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: fmt.Sprintf("%s (%d)", c.Key, c.Count)})
			}
		}
		if !salaryRestricted && len(p.Salary) > 0 {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Pay (25th percentile · median · 75th)"})
			for _, g := range p.Salary {
				for _, r := range []struct {
					name string
					r    *Range
				}{{"Current", g.Current}, {"Expected", g.Expected}} {
					if r.r != nil {
						doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: fmt.Sprintf("%s: %s · %s · %s per %s (%d candidates)",
							r.name, Money(g.Currency, r.r.Low), Money(g.Currency, r.r.Median), Money(g.Currency, r.r.High), g.Period, r.r.Count)})
					}
				}
			}
		}
	}
	if insight != nil && strings.TrimSpace(insight.Text) != "" {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Market insights (AI)"})
		doc = append(doc, simpledoc.FromText(insight.Text)...)
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Muted, Text: "From the AI's knowledge of the market: estimates, to check. " + insight.AskedAt.Format("2 Jan 2006")})
	}
	return doc
}
