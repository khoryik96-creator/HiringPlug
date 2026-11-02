package companies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }

func TestNormalizeTidiesDetails(t *testing.T) {
	d, errs := Normalize(Details{
		Name: "  Petronas   Digital ", Website: ptr("Petronas.COM/careers"), Country: ptr("my"),
		Industry: ptr("  Oil  and gas "), Phone: ptr("   "), About: ptr(" Line one\nline two "),
	})
	require.NoError(t, errs.Err())
	assert.Equal(t, "Petronas Digital", d.Name)
	assert.Equal(t, "client", d.Type, "default type")
	assert.Equal(t, "https://petronas.com/careers", *d.Website)
	assert.Equal(t, "MY", *d.Country)
	assert.Equal(t, "Oil and gas", *d.Industry)
	assert.Nil(t, d.Phone, "blank becomes empty")
	assert.Equal(t, "Line one\nline two", *d.About, "line breaks kept")
}

func TestNormalizeRejectsBlankNameAndBadWebsite(t *testing.T) {
	_, errs := Normalize(Details{Name: "  ", Website: ptr("not a website")})
	fields := []string{}
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	assert.ElementsMatch(t, []string{"name", "website"}, fields)
}

func TestNormalizeWebsite(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":              "https://example.com",
		"https://Example.com/":     "https://example.com",
		"http://example.com/jobs":  "http://example.com/jobs",
		"www.example.com.my/about": "https://www.example.com.my/about",
	} {
		got, ok := NormalizeWebsite(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"localhost", "ftp://example.com", "exa mple.com", "example"} {
		_, ok := NormalizeWebsite(bad)
		assert.False(t, ok, bad)
	}
}

func TestPatchOnlyChangesWhatIsSent(t *testing.T) {
	before := Details{Name: "Acme", Type: "client", City: ptr("Kuala Lumpur"), Industry: ptr("Banking")}
	p := Patch{}
	p.City.Set, p.City.Value = true, nil
	p.Type.Set, p.Type.Value = true, "prospect"
	after := p.Apply(before)
	assert.Equal(t, "Acme", after.Name)
	assert.Equal(t, "prospect", after.Type)
	assert.Nil(t, after.City)
	assert.Equal(t, "Banking", *after.Industry)
}
