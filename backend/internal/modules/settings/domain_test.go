package settings

import (
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
	require.Len(t, all, 5)
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
