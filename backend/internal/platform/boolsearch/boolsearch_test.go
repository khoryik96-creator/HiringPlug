package boolsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARecruitersBooleanBecomesATextSearch(t *testing.T) {
	cases := map[string]string{
		"SAP AND (BW OR BPC) NOT internship": "(('sap' & ('bw' | 'bpc')) & !'internship')",
		"sap bw":                             "('sap' & 'bw')",
		`"project manager" OR pm`:            "(('project' <-> 'manager') | 'pm')",
		"manag* -intern":                     "('manag':* & !'intern')",
		"java or kotlin and android":         "('java' | ('kotlin' & 'android'))",
		"node.js":                            "('node' <-> 'js')",
		"C++":                                "'c'",
		"not (sales OR retail) accountant":   "(!('sales' | 'retail') & 'accountant')",
		"O'Brien":                            "('o' <-> 'brien')",
	}
	for in, want := range cases {
		q, err := Parse(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, q.TSQuery(), in)
	}
}

func TestTheWordsToHighlightLeaveOutWhatIsExcluded(t *testing.T) {
	q, err := Parse(`SAP AND (BW OR bpc) NOT internship "Project Manager" manag* sap`)
	require.NoError(t, err)
	var got []string
	for _, term := range q.Terms {
		got = append(got, term.Text())
	}
	assert.Equal(t, []string{"sap", "bw", "bpc", "project manager", "manag*"}, got)
	q, err = Parse("NOT NOT finance")
	require.NoError(t, err)
	assert.Len(t, q.Terms, 1, "twice left out is looked for")
}

func TestAMistypedBooleanSaysWhatIsWrong(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "is empty",
		"  ,, ":            "is empty",
		"SAP AND (BW":      "( without its )",
		"SAP)":             ") without its (",
		"SAP AND":          "ends with AND",
		"OR SAP":           "OR with nothing before it",
		`"project manager`: `" without its closing "`,
		"()":               ") without its (",
	} {
		_, err := Parse(in)
		require.Error(t, err, in)
		assert.Contains(t, err.Error(), want, in)
	}
	deep := ""
	for range MaxDepth + 1 {
		deep += "("
	}
	_, err := Parse(deep + "sap")
	assert.ErrorContains(t, err, "deep")
}

func TestWhatIsLeftOutCanBeLookedForOnItsOwn(t *testing.T) {
	q, err := Parse(`SAP NOT (internship OR "part time") -intern*`)
	require.NoError(t, err)
	assert.Equal(t, "'internship' | ('part' <-> 'time') | 'intern':*", q.Excluded())
	q, err = Parse("SAP BW")
	require.NoError(t, err)
	assert.Empty(t, q.Excluded())
}
