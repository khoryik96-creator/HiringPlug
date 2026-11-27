package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTheFirstMorningOfTheMonthSaysWhatWentStale(t *testing.T) {
	kl := time.FixedZone("MYT", 8*3600)
	assert.True(t, StaleDay(time.Date(2026, 11, 30, 23, 30, 0, 0, time.UTC), kl), "already the 1st in Kuala Lumpur")
	assert.False(t, StaleDay(time.Date(2026, 12, 2, 1, 0, 0, 0, kl), kl))

	subject, body, ok := DigestWith(nil, Stale{Contacts: 12, Companies: 1}, "https://hp.example/")
	assert.True(t, ok, "sent for the stale line alone")
	assert.Equal(t, "This month: 13 records not touched for a year", subject)
	assert.Contains(t, body, "- 12 contacts\n  https://hp.example/#/contacts?stale=1")
	assert.Contains(t, body, "- 1 company\n  https://hp.example/#/companies?stale=1")

	items := []Item{{Kind: KindTask, Title: "Call Grab", Detail: "due today", Href: "#/tasks"}}
	subject, body, ok = DigestWith(items, Stale{Contacts: 2}, "https://hp.example")
	assert.True(t, ok)
	assert.Equal(t, "Today: 1 task due, 2 records not touched for a year", subject)
	assert.Less(t, strings.Index(body, "Call Grab"), strings.Index(body, "Not touched for a year"))
	assert.Less(t, strings.Index(body, "Not touched for a year"), strings.Index(body, "To choose what you get"))

	_, _, ok = DigestWith(nil, Stale{}, "https://hp.example")
	assert.False(t, ok, "nothing to say")
}
