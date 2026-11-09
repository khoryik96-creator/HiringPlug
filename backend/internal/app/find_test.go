package app_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The home page search by Kano's /find rules: several searches at once, by
// whole phone number, exact email or name (then names alike), and
// "name - company" among those who work or worked there.
func TestFindCandidates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	a.createCandidate(token, map[string]any{
		"firstName": "Ali", "lastName": "Abu", "email": "ali@example.com", "phone": "012-941 2110",
		"currentTitle": "Developer", "currentCompany": "Nintex Malaysia Sdn Bhd",
	})
	bakar := a.createCandidate(token, map[string]any{"firstName": "Ali", "lastName": "Bakar", "email": "aaa@gmail.com", "currentCompany": "Grab"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+bakar.Id+"/history", token, map[string]any{
		"version": bakar.Version, "education": []any{},
		"experience": []any{map[string]any{"title": "Cashier", "company": "KFC Holdings", "startDate": "2018-01", "endDate": "2020-06", "current": false, "description": nil}},
	}).Status)
	a.createCandidate(token, map[string]any{"firstName": "Hakim", "lastName": "Amirul", "phone": "+65 8123 4567"})
	a.createCandidate(token, map[string]any{"firstName": "Muhamad", "lastName": "Ali"})
	a.createCandidate(token, map[string]any{"firstName": "Nintexify", "lastName": "Tan", "currentCompany": "Nintexify"})

	find := func(q string) api.CandidateFind {
		res := a.request(http.MethodGet, "/v1/candidates/find?q="+url.QueryEscape(q), token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var out api.CandidateFind
		res.JSON(&out)
		return out
	}
	names := func(found []api.CandidateFound) []string {
		out := make([]string, len(found))
		for i, f := range found {
			out[i] = f.FullName
		}
		return out
	}
	one := func(q string) api.CandidateFindSearch {
		out := find(q)
		require.Len(t, out.Searches, 1)
		return out.Searches[0]
	}

	t.Run("a phone number, whole, in any format", func(t *testing.T) {
		for _, typed := range []string{"012-941 2110", "+60 12-941 2110", "60129412110", "0060 12 941 2110", "(012) 941 2110"} {
			s := one(typed)
			assert.Equal(t, api.CandidateFindSearchKindPhone, s.Kind)
			assert.Equal(t, []string{"Ali Abu"}, names(s.Items), typed)
		}
		assert.Empty(t, one("9412110").Items, "the last digits are not the number")
		assert.Equal(t, []string{"Hakim Amirul"}, names(one("8123 4567").Items), "a Singapore number typed without +65")
		assert.Empty(t, one("8123 4567").Similar, "no names alike for a phone number")
	})

	t.Run("an email, exactly", func(t *testing.T) {
		s := one("AAA@gmail.com")
		assert.Equal(t, api.CandidateFindSearchKindEmail, s.Kind)
		assert.Equal(t, []string{"Ali Bakar"}, names(s.Items))
		assert.Empty(t, one("aa@gmail.com").Items)
	})

	t.Run("name - company: current or past employer, word for word", func(t *testing.T) {
		s := one("ali - nintex")
		assert.Equal(t, "nintex", s.Company.MustGet())
		require.Equal(t, []string{"Ali Abu"}, names(s.Items))
		e := s.Items[0].Employer.MustGet()
		assert.Equal(t, "Nintex Malaysia Sdn Bhd", e.Company)
		assert.True(t, e.Current)
		assert.Equal(t, "Developer", e.Title.MustGet())

		s = one("ali - kfc")
		require.Equal(t, []string{"Ali Bakar"}, names(s.Items))
		assert.False(t, s.Items[0].Employer.MustGet().Current, "KFC is in their history")
		assert.Empty(t, one("tan - nintex").Items, "Nintexify is not Nintex")
		assert.Equal(t, []string{"Ali Abu"}, names(one("012 941 2110 - nintex").Items), "a phone number and a company")
	})

	t.Run("names, then names alike scored", func(t *testing.T) {
		assert.Equal(t, []string{"Hakim Amirul"}, names(one("amirul hakim").Items), "the words in any order")
		s := one("amirul hakeem")
		assert.Empty(t, s.Items, "misspelt")
		require.Equal(t, []string{"Hakim Amirul"}, names(s.Similar))
		assert.Greater(t, s.Similar[0].Score.MustGet(), float32(0.8))

		s = one("mohamad ali")
		require.NotEmpty(t, s.Similar)
		assert.Equal(t, "Muhamad Ali", s.Similar[0].FullName, "a spelling variant, best first")
		assert.Greater(t, s.Similar[0].Score.MustGet(), s.Similar[1].Score.MustGet()+0.2, "the others only share Ali")

		s = one("ali")
		assert.ElementsMatch(t, []string{"Ali Abu", "Ali Bakar", "Muhamad Ali"}, names(s.Items))
	})

	t.Run("several at once, each its own block, incomplete ones say so", func(t *testing.T) {
		out := find("ali - nintex, ali - kfc, a@@b, 12, Ali - nintex")
		require.Len(t, out.Searches, 4, "the same search twice is run once")
		assert.Equal(t, []string{"Ali Abu"}, names(out.Searches[0].Items))
		assert.Equal(t, []string{"Ali Bakar"}, names(out.Searches[1].Items))
		assert.Equal(t, "Enter a complete email address, for example aaa@gmail.com.", out.Searches[2].Error.MustGet())
		assert.Equal(t, "Enter a complete phone number. You can include + and a country code.", out.Searches[3].Error.MustGet())
		assert.Equal(t, "Use name - company, for example ali abu - nintex.", one("ali -").Error.MustGet())

		out = find("a, b, c, d, e, f, g, h")
		assert.Len(t, out.Searches, 6)
		assert.Equal(t, 2, out.Skipped)
	})

	t.Run("another agency's candidates are never found", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		res := a.request(http.MethodGet, "/v1/candidates/find?q="+url.QueryEscape("ali, 012-941 2110"), other, nil)
		require.Equal(t, http.StatusOK, res.Status)
		var out api.CandidateFind
		res.JSON(&out)
		for _, s := range out.Searches {
			assert.Empty(t, s.Items)
			assert.Empty(t, s.Similar)
		}
	})
}
