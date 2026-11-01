package candidates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

func TestNormalizeProfile(t *testing.T) {
	in := Profile{
		FirstName:      "  Tan   Wei ",
		LastName:       " Ming ",
		Email:          ptr("  Wei.Ming@Example.com "),
		Phone:          ptr("+65 9123 4567"),
		Nationality:    ptr("sg"),
		CurrentTitle:   ptr("   "),
		CurrentCompany: ptr(" Acme  Sdn Bhd "),
	}
	out, errs := NormalizeProfile(in, "MY")
	assert.Empty(t, errs)
	assert.Equal(t, "Tan Wei", out.FirstName)
	assert.Equal(t, "Ming", out.LastName)
	assert.Equal(t, "wei.ming@example.com", *out.Email)
	assert.Equal(t, "+6591234567", *out.Phone, "numbers with a country code keep it")
	assert.Equal(t, "SG", *out.Nationality)
	assert.Nil(t, out.CurrentTitle, "blank text becomes empty (nil), not a space")
	assert.Equal(t, "Acme Sdn Bhd", *out.CurrentCompany)
}

func TestLocalPhoneNumbersUseTheDefaultRegion(t *testing.T) {
	got, ok := NormalizePhone("012-345 6789", "MY")
	assert.True(t, ok)
	assert.Equal(t, "+60123456789", got)

	_, ok = NormalizePhone("12345", "MY")
	assert.False(t, ok)
}

func TestWorkAuthorizationRules(t *testing.T) {
	tests := []struct {
		name  string
		wa    WorkAuthorization
		valid bool
	}{
		{"local citizen", WorkAuthorization{Status: WorkAuthLocal}, true},
		{"needs a visa", WorkAuthorization{Status: WorkAuthRequiresVisa}, true},
		{"expat with RP-T", WorkAuthorization{Status: WorkAuthExpatEligible, Basis: ptr("rp_t")}, true},
		{"expat without a basis", WorkAuthorization{Status: WorkAuthExpatEligible}, false},
		{"local with a basis", WorkAuthorization{Status: WorkAuthLocal, Basis: ptr("permanent_resident")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wa := tc.wa
			_, errs := NormalizeProfile(Profile{FirstName: "A", WorkAuthorization: &wa}, "MY")
			assert.Equal(t, tc.valid, len(errs) == 0, "%v", errs)
		})
	}
}

func TestInvalidInputListsEveryProblem(t *testing.T) {
	_, errs := NormalizeProfile(Profile{FirstName: " ", Email: ptr("not-an-email"), Phone: ptr("123")}, "MY")
	var fields []string
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	assert.ElementsMatch(t, []string{"firstName", "email", "phone"}, fields)
}

func TestPatchOnlyChangesSentFields(t *testing.T) {
	before := Profile{FirstName: "Ali", Location: ptr("Penang"), Industry: ptr("Oil & Gas")}
	after := ProfilePatch{
		Location: Change[*string]{Set: true, Value: nil},               // clear
		Industry: Change[*string]{Set: true, Value: ptr("Renewables")}, // change
	}.Apply(before)
	assert.Equal(t, "Ali", after.FirstName)
	assert.Nil(t, after.Location)
	assert.Equal(t, "Renewables", *after.Industry)
}

func TestCents(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "8500": 850000, "8500.5": 850050, "8500.05": 850005, "999999999999.99": 99999999999999} {
		got, ok := Cents(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", ".5", "1.", "1.234", "-1", "+1", "1e3", "abc", "1234567890123"} {
		_, ok := Cents(bad)
		assert.False(t, ok, bad)
	}
}

func TestExpectedSalaryRangeMustNotRunBackwards(t *testing.T) {
	ok := ValidateCompensation(nil, &SalaryRange{Min: "9000", Max: ptr("9000.00"), Currency: "MYR", Period: "month"})
	assert.Empty(t, ok)
	bad := ValidateCompensation(nil, &SalaryRange{Min: "9000", Max: ptr("8999.99"), Currency: "MYR", Period: "month"})
	assert.Len(t, bad, 1)
	assert.Equal(t, "expectedSalary.max", bad[0].Field)
}
