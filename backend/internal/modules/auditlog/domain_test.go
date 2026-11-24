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

func TestTheActionFilterIsActionsOrKinds(t *testing.T) {
	actions, kinds := SplitActions(new("candidate.updated"))
	assert.Equal(t, []string{"candidate.updated"}, actions)
	assert.Empty(t, kinds)
	actions, kinds = SplitActions(new("candidate"))
	assert.Empty(t, actions)
	assert.Equal(t, []string{"candidate"}, kinds)
	actions, kinds = SplitActions(new("settings,invoice_settings,tenant.renamed,settings"))
	assert.Equal(t, []string{"tenant.renamed"}, actions)
	assert.Equal(t, []string{"settings", "invoice_settings"}, kinds)
	actions, kinds = SplitActions(nil)
	assert.Empty(t, actions)
	assert.Empty(t, kinds)
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

func TestLinesAreCountedInIDOrder(t *testing.T) {
	assert.Equal(t, []string{"cand_a", "cand_b"}, InLockOrder([]string{"cand_b", "", "cand_a", "cand_b"}))
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

// A cursor holds where the page ended, so it reads back without the line.
func TestCursorsSayWhereAPageEnded(t *testing.T) {
	at := time.Date(2026, 3, 10, 4, 5, 6, 789000, time.UTC)
	gotAt, id, ok := ReadLogCursor(LogCursor(at.In(time.FixedZone("MYT", 8*3600)), "aud_1"))
	assert.True(t, ok)
	assert.True(t, at.Equal(gotAt))
	assert.Equal(t, "aud_1", id)
	day, id, ok := ReadViewCursor(ViewCursor(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), "sv_1"))
	assert.True(t, ok)
	assert.Equal(t, "2026-03-10", day.Format(time.DateOnly))
	assert.Equal(t, "sv_1", id)
	for _, bad := range []string{"", "aud_1", "yesterday~aud_1", LogCursor(at, "sv_1")} {
		_, _, ok := ReadLogCursor(bad)
		assert.False(t, ok, bad)
	}
	for _, bad := range []string{"sv_1", "2026-13-01~sv_1", ViewCursor(at, "aud_1")} {
		_, _, ok := ReadViewCursor(bad)
		assert.False(t, ok, bad)
	}
}
