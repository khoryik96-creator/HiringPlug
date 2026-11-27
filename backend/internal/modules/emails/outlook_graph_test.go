package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutlookContinuationsStayOnTheSameFolder(t *testing.T) {
	initial := "https://graph.microsoft.com/v1.0/me/mailFolders/Ab%2FC/messages/delta"
	for _, c := range []struct {
		url string
		ok  bool
	}{
		{"https://graph.microsoft.com/v1.0/me/mailfolders('Ab%2FC')/messages/delta?$skiptoken=1", true},
		{"https://graph.microsoft.com/v1.0/me/mailfolders/Ab%2FC/messages/delta?$deltatoken=2", true},
		{"https://graph.microsoft.com/v1.0/users/0f1e2d3c-aaaa/mailFolders('Ab%2FC')/messages/delta?$deltatoken=2", true}, // the same mailbox, by its id
		{"https://graph.microsoft.com/beta/me/mailfolders/Ab%2FC/messages/delta?$deltatoken=2", false},                    // another version
		{"https://graph.microsoft.com/v1.0/users/x/mailfolders/Other/messages/delta?$deltatoken=2", false},                // another folder
		{"https://graph.microsoft.com/v1.0/me/mailfolders/ab%2FC/messages/delta?$skiptoken=1", false},
		{"https://graph.microsoft.com/v1.0/me/mailfolders/Ab%2FC/messages?$skiptoken=1", false},
		{"https://example.test/v1.0/me/mailfolders/Ab%2FC/messages/delta", false},
		{"https://someone@graph.microsoft.com/v1.0/me/mailfolders/Ab%2FC/messages/delta", false},
	} {
		assert.Equal(t, c.ok, outlookContinuation(initial, c.url), c.url)
	}
}

// "Could not be read" says what could not be read, so one failing mailbox can
// be told apart from another (and the log says the same).
func TestOutlookSaysWhatCouldNotBeRead(t *testing.T) {
	assert.Equal(t, "Microsoft's answer could not be read: it was larger than 6 MB", outlookReason(unreadable("it was larger than 6 MB")))
	assert.Equal(t, "Microsoft's answer could not be read", outlookReason(&outlookGraphError{status: 502}))
	assert.Equal(t, "Microsoft said ErrorAccessDenied", outlookReason(&outlookGraphError{status: 403, code: "ErrorAccessDenied"}))
}
