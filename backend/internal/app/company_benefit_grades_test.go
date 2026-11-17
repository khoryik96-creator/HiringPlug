package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A client's benefits can differ by grade: its grades, then a value for each
// grade on the benefits that differ; the Word file has a column per grade.
func TestCompanyBenefitsByGrade(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Contoh Bank Berhad"})
	path := "/v1/companies/" + company.Id + "/benefits"

	body := map[string]any{
		"version": 0,
		"grades":  []string{" Executive ", "Manager", "Senior  Manager"},
		"benefits": []map[string]any{
			{"key": "annual_leave", "label": "Annual leave", "value": nil, "remarks": nil, "byGrade": []map[string]any{
				{"grade": "Executive", "value": "14 days"}, {"grade": "Manager", "value": "18 days"}, {"grade": "Senior Manager", "value": "21 days"},
			}},
			{"key": "medical_leave", "label": "Medical leave", "value": "14 days", "remarks": nil},
			{"key": "car_allowance", "label": "Car allowance", "value": nil, "remarks": nil, "byGrade": []map[string]any{
				{"grade": "senior manager", "value": "RM 1,500"},
			}},
		},
		"preOffer": []map[string]any{},
	}
	res := a.request(http.MethodPut, path, token, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var b api.CompanyBenefits
	a.request(http.MethodGet, path, token, nil).JSON(&b)
	assert.Equal(t, []string{"Executive", "Manager", "Senior Manager"}, b.Grades, "tidied")
	require.NotNil(t, b.Benefits[0].ByGrade)
	assert.Equal(t, []api.CompanyBenefitGrade{{Grade: "Executive", Value: "14 days"}, {Grade: "Manager", Value: "18 days"}, {Grade: "Senior Manager", Value: "21 days"}}, *b.Benefits[0].ByGrade)
	assert.Empty(t, *b.Benefits[1].ByGrade, "the same for all")
	assert.Equal(t, "14 days", b.Benefits[1].Value.MustGet())

	t.Run("the Word file has a column per grade", func(t *testing.T) {
		got := a.request(http.MethodGet, path+"/file?format=docx", token, nil)
		require.Equal(t, http.StatusOK, got.Status)
		text := docxText(t, got.Body)
		for _, s := range []string{"Executive", "Senior Manager", "14 days", "21 days", "RM 1,500"} {
			assert.Contains(t, text, s)
		}
	})

	t.Run("a value only for the client's grades, each once", func(t *testing.T) {
		bad := map[string]any{"version": b.Version, "grades": []string{"Executive", "executive"}, "benefits": []map[string]any{}, "preOffer": []map[string]any{}}
		assert.Equal(t, []string{"grades[1]"}, fieldNames(t, a.request(http.MethodPut, path, token, bad).Error()))
		bad = map[string]any{"version": b.Version, "grades": []string{"Executive"}, "preOffer": []map[string]any{}, "benefits": []map[string]any{
			{"key": "annual_leave", "label": "Annual leave", "value": nil, "remarks": nil, "byGrade": []map[string]any{{"grade": "Director", "value": "30 days"}}},
		}}
		assert.Equal(t, []string{"benefits[0].byGrade[0].grade"}, fieldNames(t, a.request(http.MethodPut, path, token, bad).Error()))
	})
}
