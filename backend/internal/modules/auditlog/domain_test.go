package auditlog

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

func TestActorsAreNamedByKind(t *testing.T) {
	users := map[string]string{"usr_a": "Siti"}
	keys := map[string]string{"akey_a": "Payroll sync"}
	assert.Equal(t, ActorSystem, ActorKind("system"))
	assert.Equal(t, ActorAPIKey, ActorKind("akey_a"))
	assert.Equal(t, ActorUser, ActorKind("usr_a"))
	assert.Equal(t, "Automatic", ActorName("system", users, keys))
	assert.Equal(t, "Siti", ActorName("usr_a", users, keys))
	assert.Equal(t, "usr_gone", ActorName("usr_gone", users, keys), "a person who cannot be found keeps their id")
	assert.Equal(t, "Payroll sync", ActorName("akey_a", users, keys))
	assert.Equal(t, "API key", ActorName("akey_gone", users, keys))
}

func TestTheActionFilterIsOneActionOrAKind(t *testing.T) {
	one, kind := SplitAction(new("candidate.updated"))
	assert.Equal(t, "candidate.updated", *one)
	assert.Nil(t, kind)
	one, kind = SplitAction(new("candidate"))
	assert.Nil(t, one)
	assert.Equal(t, "candidate", *kind)
	one, kind = SplitAction(nil)
	assert.Nil(t, one)
	assert.Nil(t, kind)
}

func TestDaysAreTheAgencys(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	// 23:59 and 00:01 in Kuala Lumpur (UTC+8).
	assert.Equal(t, time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC), Day(time.Date(2026, 11, 24, 15, 59, 0, 0, time.UTC), kl))
	assert.Equal(t, time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC), Day(time.Date(2026, 11, 24, 16, 1, 0, 0, time.UTC), kl))

	from, to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	start, end := DayRange(&from, &to, kl)
	assert.Equal(t, time.Date(2026, 2, 28, 16, 0, 0, 0, time.UTC), start.UTC())
	assert.Equal(t, time.Date(2026, 3, 31, 16, 0, 0, 0, time.UTC), end.UTC(), "to: the whole day")
	start, end = DayRange(nil, nil, kl)
	assert.Nil(t, start)
	assert.Nil(t, end)
}

func TestEverySalaryViewKindIsKnown(t *testing.T) {
	assert.Len(t, KindLabels, len(salaryview.Kinds))
	for _, k := range salaryview.Kinds {
		assert.True(t, IsKind(k), k)
		assert.Equal(t, salaryview.Grouped(k), Grouped(k), k)
	}
	assert.False(t, IsKind("salary"))
}

func TestDistinctDropsBlanksAndRepeats(t *testing.T) {
	assert.Equal(t, []string{"b", "a"}, Distinct([]string{"b", "", "a", "b"}))
}

// An entry whose changes are longer than an Excel cell holds is cut short,
// saying so, and the workbook is still written.
func TestALongChangeFitsInAnExcelCell(t *testing.T) {
	long := strings.Repeat("ä", 40000)
	table := LogTable([]Entry{{Logged: audit.Logged{ID: "aud_1", Action: "note.updated", EntityType: "note", EntityID: "note_1",
		Changes: map[string]any{"body": map[string]any{"from": long, "to": "short"}}, CreatedAt: time.Now()}, ActorName: "Siti"}}, time.UTC)
	cell := table.Rows[0][6]
	assert.LessOrEqual(t, utf8.RuneCountInString(cell), CellLimit)
	assert.True(t, strings.HasPrefix(cell, `{"body":{"from":"ää`))
	assert.Contains(t, cell, "more characters: see the entry in the app)")
	file, err := sheet.Write(sheet.XLSX, "Audit log", table)
	require.NoError(t, err)
	assert.NotEmpty(t, file.Data)
	assert.Equal(t, "short", FitCell("short"))
}
