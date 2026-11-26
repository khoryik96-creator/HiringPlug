package emails

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
)

func draftWithBody(contentType, content string) outlookRemoteMessage {
	var m outlookRemoteMessage
	m.Body.ContentType, m.Body.Content = contentType, content
	return m
}

func TestALongForwardedThreadStaysEditable(t *testing.T) {
	// A forward quoting a long thread: more than 40,000 characters of text,
	// but formatting short enough to edit, so its quoted part can be cut.
	quoted := strings.Repeat("<p>Earlier in the thread: the candidate's notice period and start date.</p>", 900)
	view := draftView(store.EmailOutlookMessage{ID: "msg_1"}, draftWithBody("html", "<p>Hi Aina,</p>"+quoted), nil)
	assert.False(t, view.BodyTruncated, "edited here, not only in Outlook")
	require.NotNil(t, view.BodyHtml)
	assert.Contains(t, *view.BodyHtml, "Hi Aina,")
}

func TestADraftTooLongToEditHereIsLockedToOutlook(t *testing.T) {
	huge := strings.Repeat("<p>x</p>", maxDraftHTML/8+1)
	view := draftView(store.EmailOutlookMessage{ID: "msg_1"}, draftWithBody("html", huge), nil)
	assert.True(t, view.BodyTruncated)
	assert.Nil(t, view.BodyHtml)

	text := draftView(store.EmailOutlookMessage{ID: "msg_1"}, draftWithBody("text", strings.Repeat("a", 40001)), nil)
	assert.True(t, text.BodyTruncated, "plain text past what is shown")
	short := draftView(store.EmailOutlookMessage{ID: "msg_1"}, draftWithBody("text", "Hi"), nil)
	assert.False(t, short.BodyTruncated)
}
