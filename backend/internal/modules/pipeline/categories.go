package pipeline

// Stage categories: what each stage falls under, which reports count by (the
// funnel) and a candidate's stage tag shows. Each is one of the six kinds,
// which automations, offers and placements go by. Pure rules, unit tested.

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Category is one of the company's stage categories.
type Category struct {
	Key, Label, Kind string
}

// MaxCategories is the most categories a company keeps.
const MaxCategories = 40

// StandardCategories are a company's until it saves its own: one per kind,
// keyed by the kind, and three interview rounds.
func StandardCategories() []Category {
	out := []Category{}
	for _, st := range OpenStages {
		if st == Interview {
			out = append(out, Category{Interview, "1st Interview", Interview}, Category{"interview_2", "2nd Interview", Interview},
				Category{"interview_final", "Final Interview", Interview})
			continue
		}
		out = append(out, Category{st, stageLabels[st], st})
	}
	return out
}

// StandardStepsOf are the stages of a job with no template: one per
// category (the six kinds, and each interview round), keyed by the category.
func StandardStepsOf(cats []Category) []Step {
	out := make([]Step, len(cats))
	for i, c := range cats {
		out[i] = Step{Key: c.Key, Label: c.Label, Stage: c.Kind, Category: c.Key}
	}
	return out
}

// CategoryFor is the key of the category a stage of kind falls under: key
// when it is one of that kind, otherwise the kind's first.
func CategoryFor(cats []Category, kind, key string) string {
	first := ""
	for _, c := range cats {
		if c.Kind != kind {
			continue
		}
		if c.Key == key {
			return key
		}
		if first == "" {
			first = c.Key
		}
	}
	return first
}

// LabelOf is a category's name ("" when there is none with key).
func LabelOf(cats []Category, key string) string {
	for _, c := range cats {
		if c.Key == key {
			return c.Label
		}
	}
	return ""
}

var notKeyChars = regexp.MustCompile(`[^a-z0-9]+`)

func categoryKey(label string, taken map[string]bool) string {
	base := strings.Trim(notKeyChars.ReplaceAllString(strings.ToLower(label), "_"), "_")
	if len(base) > 30 {
		base = strings.TrimRight(base[:30], "_")
	}
	if base == "" {
		base = "category"
	}
	key := base
	for n := 2; taken[key]; n++ {
		key = base + "_" + strconv.Itoa(n)
	}
	return key
}

// NormalizeCategories tidies and checks the categories a company saves, in
// its order: each named (at most 40 characters, no two alike) and of an open
// kind; one kept from before (its key) keeps its kind; every kind keeps one.
// They come back grouped by kind, in the order given within a kind; a new
// one gets a key made from its name.
func NormalizeCategories(in, existing []Category) ([]Category, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if len(in) > MaxCategories {
		errs.Add("items", fmt.Sprintf("at most %d categories", MaxCategories))
		return nil, errs
	}
	before := map[string]Category{}
	for _, c := range existing {
		before[c.Key] = c
	}
	taken, names, seen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range in {
		if _, ok := before[c.Key]; ok {
			taken[c.Key] = true
		}
	}
	out := make([]Category, 0, len(in))
	for i, c := range in {
		at := func(field string) string { return fmt.Sprintf("items.%d.%s", i, field) }
		c.Label = joinSpaces(c.Label)
		switch n := len([]rune(c.Label)); {
		case n == 0:
			errs.Add(at("label"), "enter a name, e.g. 2nd Interview")
		case n > 40:
			errs.Add(at("label"), "at most 40 characters")
		case names[strings.ToLower(c.Label)]:
			errs.Add(at("label"), "another category has this name")
		}
		names[strings.ToLower(c.Label)] = true
		if stageIndex(c.Kind) < 0 {
			errs.Add(at("kind"), "must be applied, screening, submitted, interview, offer or placed")
		}
		if old, ok := before[c.Key]; ok && !seen[c.Key] {
			if old.Kind != c.Kind {
				errs.Add(at("kind"), "a category keeps its kind ("+old.Kind+"): add a new one instead")
			}
		} else {
			c.Key = categoryKey(c.Label, taken)
			taken[c.Key] = true
		}
		seen[c.Key] = true
		out = append(out, c)
	}
	for _, kind := range OpenStages {
		if !slices.ContainsFunc(out, func(c Category) bool { return c.Kind == kind }) {
			errs.Add("items", "keep at least one "+kind+" category")
		}
	}
	slices.SortStableFunc(out, func(a, b Category) int { return stageIndex(a.Kind) - stageIndex(b.Kind) })
	return out, errs
}

// Removed are the keys of before not in after.
func Removed(before, after []Category) []string {
	var out []string
	for _, c := range before {
		if !slices.ContainsFunc(after, func(d Category) bool { return d.Key == c.Key }) {
			out = append(out, c.Key)
		}
	}
	return out
}

// ResolveSteps gives each stage its category (its kind's first when it has
// none, or one that is gone), so stages always show one.
func ResolveSteps(steps []Step, cats []Category) []Step {
	out := slices.Clone(steps)
	for i := range out {
		out[i].Category = CategoryFor(cats, out[i].Stage, out[i].Category)
	}
	return out
}

// CheckStepCategories checks the category chosen for each stage is one of
// its kind (none: the kind's first), and fills it in.
func CheckStepCategories(steps []Step, cats []Category) ([]Step, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	out := slices.Clone(steps)
	for i, st := range out {
		if st.Category != "" && CategoryFor(cats, st.Stage, st.Category) != st.Category {
			errs.Add(fmt.Sprintf("steps.%d.category", i), "choose a "+st.Stage+" category")
			continue
		}
		out[i].Category = CategoryFor(cats, st.Stage, st.Category)
	}
	return out, errs
}

// AppCategory is the category an application at stage (and step, one of
// the job's steps) falls under: nil when closed.
func AppCategory(steps []Step, cats []Category, stage string, step *string) *string {
	if IsClosed(stage) {
		return nil
	}
	key := ""
	for _, st := range steps {
		if st.Stage == stage && (step == nil || st.Key == *step) {
			key = st.Category
			break
		}
	}
	k := CategoryFor(cats, stage, key)
	return &k
}
