package candidates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func s(v string) *string { return &v }

func TestANewerCVSaysWhereTheyAreNow(t *testing.T) {
	before := Profile{FirstName: "Siti", CurrentTitle: s("Accountant"), CurrentCompany: s("ABC Sdn Bhd"), Industry: s("Manufacturing"),
		Location: s("Ipoh"), Phone: s("+60123456789"), Skills: []string{"Excel"}}
	history := History{Work: []Job{{Title: s("Accountant"), Company: s("ABC Sdn Bhd"), StartDate: s("2020-01"), Current: true}}}
	cv := FromCV{
		Profile: Profile{CurrentTitle: s("Finance Manager"), CurrentCompany: s("XYZ Berhad"), Industry: s("Banking"), Location: s("Kuala Lumpur"),
			Phone: s("+60199999999"), Nationality: s("MY"), Skills: []string{"IFRS"}},
		History: History{Work: []Job{
			{Title: s("Finance Manager"), Company: s("XYZ Berhad"), StartDate: s("2025-03"), Current: true},
			{Title: s("Accountant"), Company: s("ABC Sdn Bhd"), StartDate: s("2020-01"), EndDate: s("2025-02")},
		}},
	}
	after, h := UpdateFrom(before, history, cv)
	assert.Equal(t, "Finance Manager", *after.CurrentTitle)
	assert.Equal(t, "XYZ Berhad", *after.CurrentCompany)
	assert.Equal(t, "Banking", *after.Industry)
	assert.Equal(t, "Kuala Lumpur", *after.Location)
	assert.Equal(t, "+60123456789", *after.Phone, "contact details are only filled in, never replaced")
	assert.Equal(t, "MY", *after.Nationality, "an empty field is filled")
	assert.Equal(t, []string{"Excel", "IFRS"}, after.Skills)
	assert.Len(t, h.Work, 2)

	t.Run("a CV that names no employer leaves the current job", func(t *testing.T) {
		after, h := UpdateFrom(before, history, FromCV{Profile: Profile{Industry: s("  ")}})
		assert.Equal(t, "Accountant", *after.CurrentTitle)
		assert.Equal(t, "Manufacturing", *after.Industry, "a blank value replaces nothing")
		assert.Len(t, h.Work, 1)
	})
	t.Run("a shorter history is not replaced", func(t *testing.T) {
		long := History{Work: append(cv.History.Work, Job{Title: s("Clerk"), StartDate: s("2018")})}
		_, h := UpdateFrom(before, long, FromCV{History: History{Work: cv.History.Work[:1]}})
		assert.Len(t, h.Work, 3)
	})
	t.Run("an old CV found later only fills empty fields", func(t *testing.T) {
		now := History{Work: cv.History.Work}
		old := FromCV{Profile: Profile{CurrentTitle: s("Clerk"), CurrentCompany: s("Old Co"), Nationality: s("MY")},
			History: History{Work: []Job{{Title: s("Clerk"), Company: s("Old Co"), StartDate: s("2018"), Current: true}}}}
		after, h := UpdateFrom(Profile{CurrentTitle: s("Finance Manager")}, now, old)
		assert.Equal(t, "Finance Manager", *after.CurrentTitle)
		assert.Equal(t, "Old Co", *after.CurrentCompany, "empty, so filled")
		assert.Equal(t, now.Work, h.Work)
	})
}
