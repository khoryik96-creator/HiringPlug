package companies

import (
	"strings"
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

func TestReadingACompanyWebsite(t *testing.T) {
	page := `<html><head><title>Contoh</title>
<meta name="description" content="Contoh is a Malaysian accounting firm &amp; payroll provider.">
<meta property="og:site_name" content="Contoh Group">
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[
 {"@type":"WebSite","name":"Contoh website"},
 {"@type":["Organization","AccountingService"],"name":"Contoh","legalName":"Contoh Sdn. Bhd.","url":"https://www.contoh.com.my/",
  "telephone":"+603-2141 0000","numberOfEmployees":{"@type":"QuantitativeValue","minValue":51,"maxValue":200},
  "address":{"@type":"PostalAddress","streetAddress":"Level 12, Menara Contoh, Jalan Ampang","postalCode":"50450","addressLocality":"Kuala Lumpur","addressCountry":"Malaysia"}}
]}</script></head>
<body><a href="/hubungi-kami">Hubungi</a> <a href="https://www.contoh.com.my/contact-us#form">Contact us</a>
<footer>© 2026 Contoh Sdn. Bhd. Company No. 201901012345 (1234567-X). All rights reserved.</footer></body></html>`
	got := map[string]Suggestion{}
	for _, s := range ReadSite([]byte(page), "https://www.contoh.com.my/") {
		got[s.Field] = s
	}
	assert.Equal(t, "Contoh Sdn. Bhd.", got["name"].Value, "the legal name first")
	assert.Equal(t, "+603-2141 0000", got["phone"].Value)
	assert.Equal(t, "Level 12, Menara Contoh, Jalan Ampang, 50450", got["address"].Value)
	assert.Equal(t, "Kuala Lumpur", got["city"].Value)
	assert.Equal(t, "MY", got["country"].Value)
	assert.Equal(t, "medium", got["size"].Value)
	assert.Equal(t, "Contoh is a Malaysian accounting firm & payroll provider.", got["about"].Value)
	assert.Equal(t, "201901012345 (1234567-X)", got["registrationNumber"].Value)
	assert.Contains(t, got["registrationNumber"].Source, `Company No. 201901012345 (1234567-X)`)
	assert.Equal(t, "schema.org data on https://www.contoh.com.my/", got["phone"].Source)
	assert.Equal(t, "https://www.contoh.com.my/hubungi-kami", ContactPage([]byte(page), "https://www.contoh.com.my/"), "Malay contact pages too")
}

func TestReadingASiteWithoutData(t *testing.T) {
	page := `<html><head><meta property="og:site_name" content="Maju Logistics"></head>
<body><a href="tel:+60%203-1234%205678">Call</a> Maju Logistics Sdn Bhd (987654-W) <a href="https://other.com/contact">x</a></body></html>`
	got := map[string]string{}
	for _, s := range ReadSite([]byte(page), "https://maju.my/") {
		got[s.Field] = s.Value
	}
	assert.Equal(t, map[string]string{"name": "Maju Logistics", "phone": "+60 3-1234 5678", "registrationNumber": "987654-W"}, got)
	assert.Empty(t, ContactPage([]byte(page), "https://maju.my/"), "only the company's own site")
}

func TestSuggestionsFitTheDatabase(t *testing.T) {
	long := strings.Repeat("é", 5000)
	page := `<html><head><meta name="description" content="` + long + `"></head></html>`
	found := ReadSite([]byte(page), "https://contoh.my/hubungi?ref="+strings.Repeat("x", 600))
	require.Len(t, found, 1)
	assert.Len(t, []rune(found[0].Value), 4000, "values up to 4,000 characters")
	assert.Len(t, []rune(found[0].Source), 500, "sources up to 500 characters")
}

func TestSizesAndCountries(t *testing.T) {
	assert.Equal(t, "micro", SizeOf(float64(8)))
	assert.Equal(t, "enterprise", SizeOf("12,000"))
	assert.Equal(t, "large", SizeOf(map[string]any{"value": float64(500)}))
	assert.Empty(t, SizeOf(nil))
	assert.Equal(t, "SG", CountryCode("Singapore"))
	assert.Equal(t, "MY", CountryCode("my"))
	assert.Empty(t, CountryCode("Atlantis"))
	found := Merge([]Suggestion{{Field: "name", Value: "A"}}, []Suggestion{{Field: "name", Value: "B"}, {Field: "phone", Value: "1"}})
	assert.Len(t, found, 2, "a later page only adds what is missing")
}
