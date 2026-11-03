package leads

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSources(t *testing.T) {
	for in, want := range map[string]string{
		"acme": "acme", "https://boards.greenhouse.io/acme": "acme", "https://boards-api.greenhouse.io/v1/boards/acme/jobs": "acme",
		"https://jobs.lever.co/acme/": "acme", "https://jobs.ashbyhq.com/acme": "acme",
	} {
		id, _, errs := NormalizeSource(Greenhouse, in, "Acme")
		assert.Empty(t, errs, in)
		assert.Equal(t, want, id, in)
	}
	_, _, errs := NormalizeSource(Lever, "acme corp!", "")
	assert.Len(t, errs, 2, "a board name, and the company's name")

	id, name, errs := NormalizeSource(CareersPage, " https://www.contoh.com.my/careers ", "")
	assert.Empty(t, errs)
	assert.Equal(t, "https://www.contoh.com.my/careers", id)
	assert.Equal(t, "contoh.com.my", name, "named after its site until the markup names it")
	_, _, errs = NormalizeSource(CareersPage, "javascript:alert(1)", "x")
	assert.Equal(t, "identifier", errs[0].Field)
}

func TestDiff(t *testing.T) {
	sync := Diff(
		[]Existing{{ID: "lead_a", ExternalID: "1"}, {ID: "lead_b", ExternalID: "2"}, {ID: "lead_c", ExternalID: "3", Closed: true}},
		[]Posting{{ExternalID: "1", Title: "Accountant"}, {ExternalID: "3", Title: "Auditor"}, {ExternalID: "4", Title: "Analyst"}, {ExternalID: "4", Title: "Analyst"}},
	)
	assert.Equal(t, []string{"4"}, []string{sync.New[0].ExternalID})
	assert.Len(t, sync.New, 1, "each opening once")
	assert.Contains(t, sync.Seen, "lead_a")
	assert.Contains(t, sync.Seen, "lead_c", "listed again: reopened")
	assert.Equal(t, []string{"lead_b"}, sync.Closed, "no longer listed")
}

func TestFeeds(t *testing.T) {
	gh, err := ParseGreenhouse([]byte(`{"jobs":[{"id":4012345,"title":" Accountant ","location":{"name":"Kuala Lumpur"},"absolute_url":"https://boards.greenhouse.io/acme/jobs/4012345","first_published":"2026-10-01T10:00:00-04:00"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "4012345", gh[0].ExternalID)
	assert.Equal(t, "Accountant", gh[0].Title)
	assert.Equal(t, "Kuala Lumpur", *gh[0].Location)
	assert.Equal(t, 2026, gh[0].PostedAt.Year())

	lv, err := ParseLever([]byte(`[{"id":"a1b2","text":"Auditor","categories":{"location":"Penang"},"hostedUrl":"https://jobs.lever.co/acme/a1b2","createdAt":1790000000000}]`))
	require.NoError(t, err)
	assert.Equal(t, "Penang", *lv[0].Location)
	assert.Equal(t, time.UnixMilli(1790000000000).UTC(), *lv[0].PostedAt)

	ab, err := ParseAshby([]byte(`{"jobs":[{"id":"c3","title":"Analyst","location":"Remote","jobUrl":"https://jobs.ashbyhq.com/acme/c3","publishedAt":"2026-10-02T00:00:00.000Z"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "https://jobs.ashbyhq.com/acme/c3", *ab[0].URL)

	sr, err := ParseSmartRecruiters([]byte(`{"content":[{"id":"744","name":"Engineer","releasedDate":"2026-10-03T00:00:00.000Z","location":{"city":"Kuala Lumpur","country":"my"}}]}`), "Acme")
	require.NoError(t, err)
	assert.Equal(t, "Kuala Lumpur, MY", *sr[0].Location)
	assert.Equal(t, "https://jobs.smartrecruiters.com/Acme/744", *sr[0].URL)

	_, err = ParseGreenhouse([]byte(`<html>not found</html>`))
	assert.Error(t, err)
}

func TestCareersPageMarkup(t *testing.T) {
	page := `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"Organization","name":"Contoh"}</script>
<script type='application/ld+json' data-x="1">
{"@context":"https://schema.org","@graph":[
 {"@type":"JobPosting","title":"Account Executive","identifier":{"@type":"PropertyValue","value":"AE-7"},"datePosted":"2026-10-05",
  "hiringOrganization":{"@type":"Organization","name":"Contoh Berhad"},
  "jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Kuala Lumpur","addressCountry":"MY"}}},
 {"@type":"JobPosting","title":"HR Executive &amp; Admin","url":"https://contoh.com.my/careers/hr"}
]}</script>
<script type="application/ld+json">{ broken</script></head></html>`
	got := ParseJobPostings([]byte(page), "https://contoh.com.my/careers")
	require.Len(t, got, 2)
	assert.Equal(t, "AE-7", got[0].ExternalID)
	assert.Equal(t, "Contoh Berhad", got[0].Company)
	assert.Equal(t, "Kuala Lumpur, MY", *got[0].Location)
	assert.Equal(t, "https://contoh.com.my/careers", *got[0].URL, "the page, when a posting has no address of its own")
	assert.Equal(t, "HR Executive & Admin", got[1].Title)
	assert.Equal(t, "https://contoh.com.my/careers/hr", got[1].ExternalID)
}
