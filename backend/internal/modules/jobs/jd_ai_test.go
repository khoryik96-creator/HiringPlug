package jobs

import (
	"testing"
	"time"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// A description as clients send it: their letterhead first, no labels.
const letterhead = `MAJU FOODS BERHAD
Driving Growth Across Asia

We are hiring 2 positions for our Shah Alam plant.

Senior Production Planner

The role
- Plan weekly production for 3 lines
- Work with procurement and sales

Pay: RM 7,000 - 8,500 a month`

func TestTheLetterheadIsTheCompanyNotTheTitle(t *testing.T) {
	d := ReadJD(letterhead, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "MAJU FOODS BERHAD", d.CompanyName)
	assert.Equal(t, "Senior Production Planner", d.Title, "not the letterhead, nor the slogan under it")
	assert.Equal(t, 2, *d.Openings, `"2 positions"`)
	assert.Equal(t, "Shah Alam", d.Location)

	labelled := ReadJD("Finance Manager\nCompany: Contoh Holdings\nNo. of vacancies: 3\nAbout Contoh Holdings", time.Now())
	assert.Equal(t, "Contoh Holdings", labelled.CompanyName)
	assert.Equal(t, 3, *labelled.Openings)
	hiring := ReadJD("Banking Made Simple\nWe are hiring 3 Data Engineers for our KL hub.\nData Engineer", time.Now())
	assert.Equal(t, "Data Engineer", hiring.Title)
	assert.Equal(t, 3, *hiring.Openings, `"hiring 3 Data Engineers"`)
	about := ReadJD("Finance Manager\n\nAbout Maju Foods Berhad\nA leading FMCG maker.", time.Now())
	assert.Equal(t, "Maju Foods Berhad", about.CompanyName, `"About <company>"`)
}

func TestTheAIsReadingIsUsedWhereTheDescriptionSaysIt(t *testing.T) {
	rules := ReadJD(letterhead, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	ai := api.JdAiDetails{
		Title: nullable.NewNullableWithValue("Senior Production Planner"), CompanyName: nullable.NewNullableWithValue("Maju Foods Berhad"),
		Location: nullable.NewNullableWithValue("Shah Alam"), Country: nullable.NewNullableWithValue("MY"),
		WorkType:        nullable.NewNullableWithValue(api.WorkTypeFullTime),
		WorkArrangement: nullable.NewNullableWithValue(api.WorkArrangementOnsite), Openings: nullable.NewNullableWithValue(2),
	}
	d := WithAI(letterhead, rules, ai)
	assert.Equal(t, "Senior Production Planner", d.Title)
	assert.Equal(t, "Maju Foods Berhad", d.CompanyName, "the description has it (case aside)")
	assert.Equal(t, "MY", d.Country)
	assert.Equal(t, "full_time", d.WorkType)
	assert.Equal(t, "onsite", d.WorkArrangement)
	assert.Equal(t, rules.SalaryText, d.SalaryText, "pay stays the rules': the AI never sees it")

	made := WithAI(letterhead, rules, api.JdAiDetails{Title: nullable.NewNullableWithValue("Head of Planning"),
		CompanyName: nullable.NewNullableWithValue("Imaginary Sdn Bhd"), Location: nullable.NewNullNullable[string](),
		Country: nullable.NewNullNullable[string](), WorkType: nullable.NewNullNullable[api.WorkType](),
		WorkArrangement: nullable.NewNullNullable[api.WorkArrangement](), Openings: nullable.NewNullNullable[int]()})
	assert.Equal(t, rules.Title, made.Title, "a title the description does not have is not used")
	assert.Equal(t, rules.CompanyName, made.CompanyName)
}
