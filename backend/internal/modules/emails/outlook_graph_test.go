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
		{"https://graph.microsoft.com/v1.0/me/mailfolders/ab%2FC/messages/delta?$skiptoken=1", false},
		{"https://graph.microsoft.com/v1.0/me/mailfolders/Ab%2FC/messages?$skiptoken=1", false},
		{"https://example.test/v1.0/me/mailfolders/Ab%2FC/messages/delta", false},
		{"https://someone@graph.microsoft.com/v1.0/me/mailfolders/Ab%2FC/messages/delta", false},
	} {
		assert.Equal(t, c.ok, outlookContinuation(initial, c.url), c.url)
	}
}
