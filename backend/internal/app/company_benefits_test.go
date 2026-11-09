package app_test

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Every list in Settings can be saved: the call lists and the benefits
// template could not (the database knew only the first five lists).
func TestEveryListCanBeSaved(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	owner := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	var all api.FieldSettings
	a.request(http.MethodGet, "/v1/settings/fields", owner, nil).JSON(&all)
	for _, l := range all.Lists {
		options := make([]map[string]any, len(l.Options))
		for i, o := range l.Options {
			options[i] = map[string]any{"key": o.Key, "label": o.Label, "hidden": o.Hidden}
			if h, err := o.Hint.Get(); err == nil {
				options[i]["hint"] = h
			}
		}
		res := a.request(http.MethodPut, "/v1/settings/fields/lists/"+string(l.Name), owner, map[string]any{"options": options})
		assert.Equal(t, http.StatusOK, res.Status, "%s: %s", l.Name, res.Body)
	}
}

func TestCompanyBenefits(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner, recruiter, viewer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	path := "/v1/companies/" + company.Id + "/benefits"

	t.Run("the template is the agency's, Hyppies' to start with, with hints", func(t *testing.T) {
		var all api.FieldSettings
		a.request(http.MethodGet, "/v1/settings/fields", recruiter, nil).JSON(&all)
		var benefits api.FieldList
		for _, l := range all.Lists {
			if l.Name == api.FieldListNameCompanyBenefits {
				benefits = l
			}
		}
		require.True(t, benefits.Hints)
		assert.Equal(t, "Annual leave", benefits.Options[1].Label)
		assert.Contains(t, benefits.Options[3].Hint.MustGet(), "immediate family")

		options := []map[string]any{}
		for _, o := range benefits.Options {
			options = append(options, map[string]any{"key": o.Key, "label": o.Label})
		}
		options = append(options, map[string]any{"label": "Car allowance", "hint": "Amount, and from which grade"})
		res := a.request(http.MethodPut, "/v1/settings/fields/lists/company_benefits", owner, map[string]any{"options": options})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var saved api.FieldList
		res.JSON(&saved)
		last := saved.Options[len(saved.Options)-1]
		assert.Equal(t, "car_allowance", last.Key)
		assert.Equal(t, "Amount, and from which grade", last.Hint.MustGet())
	})

	var b api.CompanyBenefits
	a.request(http.MethodGet, path, viewer, nil).JSON(&b)
	assert.Equal(t, 0, b.Version, "nothing saved yet")
	assert.Empty(t, b.Benefits)

	body := map[string]any{
		"version": 0,
		"benefits": []map[string]any{
			{"key": "annual_leave", "label": "Annual leave", "value": " 18 days ", "remarks": "21 after 3 years"},
			{"key": "fixed_bonus", "label": "Fixed bonus", "value": "13th month", "remarks": nil},
			{"key": "car_allowance", "label": "Car allowance", "value": nil, "remarks": nil},
		},
		"preOffer": []map[string]any{
			{"key": "reference_check", "label": "Reference check", "applies": true, "remarks": "2 referees: a superior and a peer"},
			{"key": "health_screening", "label": "Health screening", "applies": false, "remarks": nil},
			{"key": "background_check", "label": "Background check", "applies": nil, "remarks": nil},
		},
	}
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, path, viewer, body).Status)
	res := a.request(http.MethodPut, path, recruiter, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&b)
	assert.Equal(t, 1, b.Version)
	assert.Equal(t, "18 days", b.Benefits[0].Value.MustGet(), "trimmed")
	assert.True(t, b.Benefits[2].Value.IsNull())
	assert.True(t, b.PreOffer[0].Applies.MustGet())
	assert.False(t, b.PreOffer[1].Applies.MustGet())
	assert.True(t, b.PreOffer[2].Applies.IsNull(), "not known yet")

	var again api.CompanyBenefits
	a.request(http.MethodGet, path, viewer, nil).JSON(&again)
	assert.Equal(t, b, again)

	t.Run("someone else's save is not overwritten", func(t *testing.T) {
		res := a.request(http.MethodPut, path, recruiter, body) // still version 0
		assert.Equal(t, http.StatusConflict, res.Status)
	})

	t.Run("each item once, named", func(t *testing.T) {
		bad := map[string]any{"version": 1, "preOffer": []map[string]any{}, "benefits": []map[string]any{
			{"key": "a", "label": "A", "value": nil, "remarks": nil}, {"key": "a", "label": " ", "value": nil, "remarks": nil},
		}}
		res := a.request(http.MethodPut, path, recruiter, bad)
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.ElementsMatch(t, []string{"benefits[1].key", "benefits[1].label"}, fieldNames(t, res.Error()))
	})

	t.Run("downloads as Word, in two tables", func(t *testing.T) {
		res := a.request(http.MethodGet, path+"/file?format=docx", viewer, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Contains(t, res.Header.Get("Content-Disposition"), "Maju Foods Berhad - Benefits.docx")
		zr, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
		require.NoError(t, err)
		var doc string
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				r, err := f.Open()
				require.NoError(t, err)
				data, _ := io.ReadAll(r)
				doc = string(data)
			}
		}
		for _, want := range []string{"Annual leave", "18 days", "21 after 3 years", "Pre-offer process", "2 referees: a superior and a peer", "Yes", "No"} {
			assert.Contains(t, doc, want)
		}
		assert.Equal(t, 2, bytes.Count([]byte(doc), []byte("<w:tbl>")))
		pdf := a.request(http.MethodGet, path+"/file?format=pdf", viewer, nil)
		require.Equal(t, http.StatusOK, pdf.Status)
		assert.True(t, bytes.HasPrefix(pdf.Body, []byte("%PDF-")))
	})

	t.Run("another agency cannot see them", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path, other, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPut, path, other, body).Status)
	})
}
