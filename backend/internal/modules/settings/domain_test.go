package settings

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	e, ok := apperr.As(err)
	require.True(t, ok, "%v", err)
	fields, _ := e.Details["fields"].(apperr.FieldErrors)
	var out []string
	for _, f := range fields {
		out = append(out, f.Field)
	}
	return out
}

func builtIns(name string) []OptionInput {
	d, _ := def(name)
	var out []OptionInput
	for _, o := range d.defaults {
		out = append(out, OptionInput{Key: o.Key, Label: o.Label})
	}
	return out
}

func TestDefaultsUntilACompanySaves(t *testing.T) {
	all := Lists(map[string][]Option{"candidate_source": {{Key: "job_fair", Label: "Job fair"}, {Key: "linkedin", Label: "LinkedIn", Hidden: true}}})
	require.Len(t, all, 10)
	assert.Equal(t, []string{"RFL", "LF"}, labelsOf(all[5].Options), "a screening call's items, ready to change")
	assert.Equal(t, "candidate_source", all[0].Name)
	assert.Equal(t, []Option{{Key: "job_fair", Label: "Job fair"}, {Key: "linkedin", Label: "LinkedIn", Hidden: true, BuiltIn: true}}, all[0].Options)
	assert.Len(t, all[2].Options, 10, "Kano's industries")
	assert.True(t, all[3].Fixed)
}

func TestOpenListsTakeNewOptions(t *testing.T) {
	in := append(builtIns("candidate_source"), OptionInput{Label: " Job  Fair 2026 "}, OptionInput{Label: "job FAIR 2026"})
	_, err := CheckList("candidate_source", in)
	assert.Equal(t, []string{"options[7].label"}, fieldsOf(t, err), "labels are unique, ignoring case")

	got, err := CheckList("candidate_source", append(builtIns("candidate_source"), OptionInput{Label: "Job Fair 2026"}, OptionInput{Label: "Job-fair 2026", Hidden: true}))
	require.NoError(t, err)
	assert.Equal(t, Option{Key: "job_fair_2026", Label: "Job Fair 2026"}, got[6])
	assert.Equal(t, Option{Key: "job_fair_2026_2", Label: "Job-fair 2026", Hidden: true}, got[7], "keys stay unique")

	_, err = CheckList("candidate_source", builtIns("candidate_source")[1:])
	assert.Equal(t, []string{"options"}, fieldsOf(t, err), "built-in options are hidden, not removed")
	_, err = CheckList("candidate_source", append(builtIns("candidate_source"), OptionInput{Label: "!!!"}))
	assert.Equal(t, []string{"options[6].label"}, fieldsOf(t, err))
	_, err = CheckList("candidate_source", append(builtIns("candidate_source"), OptionInput{Key: "Job Fair", Label: "Job fair"}))
	assert.Equal(t, []string{"options[6].key"}, fieldsOf(t, err), "a key no candidate could be given")
}

func TestNoticePeriodsAndIndustries(t *testing.T) {
	got, err := CheckList("notice_period", append(builtIns("notice_period"), OptionInput{Key: "6_month", Label: "6 months"}))
	require.NoError(t, err)
	assert.Equal(t, "6_month", got[len(got)-1].Key)
	_, err = CheckList("notice_period", append(builtIns("notice_period"), OptionInput{Key: "soon", Label: "Soon"}, OptionInput{Key: "400_day", Label: "400 days"}))
	assert.Equal(t, []string{"options[6].key", "options[7].key"}, fieldsOf(t, err))

	got, err = CheckList("industry", []OptionInput{{Key: "ignored", Label: "Fintech"}, {Label: "Banking"}})
	require.NoError(t, err, "industries are words: built-in ones may go")
	assert.Equal(t, []Option{{Key: "Fintech", Label: "Fintech"}, {Key: "Banking", Label: "Banking"}}, got)
}

func TestFixedListsAreRenamedNotExtended(t *testing.T) {
	in := builtIns("work_type")
	in[0].Label, in[1].Hidden = "Permanent", true
	got, err := CheckList("work_type", in)
	require.NoError(t, err)
	assert.Equal(t, Option{Key: "full_time", Label: "Permanent"}, got[0])
	_, err = CheckList("work_type", append(builtIns("work_type"), OptionInput{Key: "gig", Label: "Gig"}))
	assert.Equal(t, []string{"options[5].key"}, fieldsOf(t, err))
	_, err = CheckList("job_status", nil)
	assert.Error(t, err)
}

func TestMandatoryFields(t *testing.T) {
	rules := map[string]bool{"candidate/email": true, "candidate/source": false, "job/location": true, "candidate/firstName": false}
	assert.Equal(t, []string{"email"}, MandatoryOn("candidate", rules))
	assert.Equal(t, []string{"location"}, MandatoryOn("job", rules))
	for _, f := range Fields(rules) {
		if f.Field == "firstName" {
			assert.True(t, f.Mandatory && f.AlwaysReqd, "the app always needs a first name")
		}
	}
	assert.NoError(t, CheckRules([]RuleInput{{Form: "candidate", Field: "phone", Mandatory: true}}))
	err := CheckRules([]RuleInput{{Form: "candidate", Field: "salary", Mandatory: true}, {Form: "job", Field: "title", Mandatory: false}})
	assert.Equal(t, []string{"fields[0].field", "fields[1].mandatory"}, fieldsOf(t, err))
}

func labelsOf(options []Option) []string {
	out := make([]string, len(options))
	for i, o := range options {
		out[i] = o.Label
	}
	return out
}

func TestCallItemsCanBeAddedRenamedAndHidden(t *testing.T) {
	in := []OptionInput{{Key: "referee", Label: "Referee"}, {Key: "rehire", Label: "Rehire?", Hidden: true}, {Label: "Attendance and punctuality"}}
	for _, k := range []string{"relationship", "role", "strengths", "improve", "reason_left"} {
		in = append(in, OptionInput{Key: k, Label: k})
	}
	got, err := CheckList("reference_check", in)
	require.NoError(t, err)
	assert.Equal(t, "attendance_and_punctuality", got[2].Key)
	assert.True(t, got[1].Hidden)
	_, err = CheckList("screening_call", []OptionInput{{Key: "rfl", Label: "RFL"}})
	require.Error(t, err, "the app's items are hidden, not removed")
}

func TestBenefitItemsCarryHints(t *testing.T) {
	all := Lists(nil)
	i := slices.IndexFunc(all, func(l List) bool { return l.Name == "company_benefits" })
	require.GreaterOrEqual(t, i, 0)
	assert.True(t, all[i].Hints)
	assert.Equal(t, "Annual leave", all[i].Options[1].Label, "Hyppies' template")
	assert.NotEmpty(t, all[i].Options[1].Hint)

	var in []OptionInput
	for _, o := range all[i].Options {
		in = append(in, OptionInput{Key: o.Key, Label: o.Label, Hint: o.Hint})
	}
	in = append(in, OptionInput{Label: "Car allowance", Hint: "  Amount   and   grade  "})
	got, err := CheckList("company_benefits", in)
	require.NoError(t, err)
	assert.Equal(t, "car_allowance", got[len(got)-1].Key)
	assert.Equal(t, "Amount and grade", got[len(got)-1].Hint)

	in[len(in)-1].Hint = strings.Repeat("x", MaxHint+1)
	_, err = CheckList("company_benefits", in)
	require.Error(t, err)

	plain, err := CheckList("screening_call", []OptionInput{{Key: "rfl", Label: "RFL", Hint: "ignored"}, {Key: "lf", Label: "LF"}})
	require.NoError(t, err)
	assert.Empty(t, plain[0].Hint, "only lists with hints keep one")
}

func TestCompanyCategoriesCarryAColour(t *testing.T) {
	all := Lists(nil)
	cats := all[len(all)-1]
	require.Equal(t, "company_category", cats.Name)
	assert.True(t, cats.Colors)
	assert.Equal(t, []string{"Signed client", "Prospective client", "Unknown", "VIP client"}, labelsOf(cats.Options))
	assert.Equal(t, "#16a34a", cats.Options[0].Color)

	in := append(builtIns("company_category"), OptionInput{Label: "Key account", Color: "#7C3AED"}, OptionInput{Label: "Dormant"})
	got, err := CheckList("company_category", in)
	require.NoError(t, err)
	assert.Equal(t, Option{Key: "key_account", Label: "Key account", Color: "#7c3aed"}, got[4], "named and coloured by the agency")
	assert.Equal(t, "#6b7280", got[5].Color, "grey until a colour is chosen")
	assert.Equal(t, "#16a34a", got[0].Color, "a built-in one keeps its own")
	_, err = CheckList("company_category", append(builtIns("company_category"), OptionInput{Label: "Hot", Color: "red"}))
	require.Error(t, err)
	got, err = CheckList("candidate_source", append(builtIns("candidate_source"), OptionInput{Label: "Job fair", Color: "#16a34a"}))
	require.NoError(t, err)
	assert.Empty(t, got[len(got)-1].Color, "only lists with colours keep one")
}
