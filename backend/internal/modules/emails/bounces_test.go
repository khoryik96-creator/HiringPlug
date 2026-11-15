package emails

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBouncedRecipientsAreTheAddressesABounceNoticeNames(t *testing.T) {
	recipients := []string{"Ana@Maju.test", "nur@maju.test", "ana@maju.test", " "}
	cases := []struct {
		name, subject, body string
		want                []string
	}{
		{"Exchange", "Undeliverable: Shortlist", "<p>Delivery has failed to these recipients:</p><a href=\"mailto:ana@maju.test\">ana@maju.test</a>", []string{"ana@maju.test"}},
		{"Gmail", "Delivery Status Notification (Failure)", "Address not found. Your message wasn't delivered to NUR@maju.test because the address couldn't be found.", []string{"nur@maju.test"}},
		{"Postfix", "Undelivered Mail Returned to Sender", "<ana@maju.test>: host mx.maju.test said: 550 5.1.1 User unknown\n<nur@maju.test>: 550", []string{"ana@maju.test", "nur@maju.test"}},
		{"Exim", "Mail delivery failed: returning message to sender", "  ana@maju.test\n    No such user here", []string{"ana@maju.test"}},
		{"an address at the end of a sentence", "Undeliverable: Hi", "Could not deliver to ana@maju.test.", []string{"ana@maju.test"}},
		{"entity-encoded HTML", "Undeliverable: Hi", "&lt;ana@maju.test&gt;", []string{"ana@maju.test"}},
		{"not a bounce notice", "Re: Undeliverable?", "ana@maju.test", nil},
		{"an out-of-office", "Automatic reply: Shortlist", "ana@maju.test is away", nil},
		{"a longer address containing it", "Undeliverable: Hi", "joana@maju.test, ana@maju.test.my, ana@maju.testing", nil},
		{"none of the recipients", "Undeliverable: Hi", "someone@else.test", nil},
		{"Exchange, quoting the original headers", "Undeliverable: Shortlist",
			"<p>Your message to nur@maju.test couldn't be delivered.</p><p>Diagnostic information for administrators:</p><p>nur@maju.test<br>Remote Server returned '550 5.1.1'</p><p>Original message headers:</p><pre>From: recruiter@agency.test\nTo: Ana@maju.test, nur@maju.test</pre>",
			[]string{"nur@maju.test"}},
		{"Gmail, quoting the original message", "Delivery Status Notification (Failure)",
			"Your message wasn't delivered to nur@maju.test.\n\n----- Original message -----\nTo: ana@maju.test, nur@maju.test", []string{"nur@maju.test"}},
		{"Exim, with a copy of the message", "Mail delivery failed: returning message to sender",
			"  nur@maju.test\n    No such user\n------ This is a copy of the message, including all the headers. ------\nCc: ana@maju.test", []string{"nur@maju.test"}},
		{"a header line quoted on its own", "Undeliverable: Hi", "nur@maju.test: 550 user unknown\nTo: ana@maju.test", []string{"nur@maju.test"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, BouncedRecipients(c.subject, c.body, recipients))
		})
	}
}

func TestContainsAddressMatchesWholeAddressesOnly(t *testing.T) {
	assert.True(t, containsAddress("ana@x.com", "ana@x.com"))
	assert.True(t, containsAddress("to: ana@x.com, nur@x.com", "nur@x.com"))
	assert.True(t, containsAddress("joana@x.com then ana@x.com", "ana@x.com"), "a later whole match counts")
	assert.False(t, containsAddress("joana@x.com", "ana@x.com"))
	assert.False(t, containsAddress("ana@x.com.my", "ana@x.com"))
	assert.False(t, containsAddress("ana@x.community", "ana@x.com"))
	assert.False(t, containsAddress("x.ana@x.com", "ana@x.com"))
}

func TestBounceNoticesReadsSubjectsPageByPageThenOnlyTheNoticesText(t *testing.T) {
	var server *httptest.Server
	var bodies []string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "Bearer synthetic-access", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/me/mailFolders/inbox/messages":
			assert.Equal(t, "id,subject", r.URL.Query().Get("$select"), "subjects only: bodies can be large")
			switch r.URL.Query().Get("$skip") {
			case "":
				assert.Equal(t, "receivedDateTime ge 2026-11-14T01:00:00Z", r.URL.Query().Get("$filter"))
				_, _ = w.Write([]byte(`{"value":[{"id":"m1","subject":"Weekly newsletter"},{"id":"m/2","subject":"Undeliverable: Hi"}],
					"@odata.nextLink":"` + server.URL + `/me/mailFolders/inbox/messages?%24select=id%2Csubject&%24skip=2"}`))
			case "2":
				_, _ = w.Write([]byte(`{"value":[{"id":"m3","subject":"Mail delivery failed"}],"@odata.nextLink":"https://elsewhere.test/steal?$skip=3"}`))
			default:
				t.Errorf("unexpected page %s", r.URL.RawQuery)
			}
		default:
			id := r.URL.Path[len("/me/messages/"):]
			bodies = append(bodies, id)
			assert.Equal(t, "body", r.URL.Query().Get("$select"))
			_, _ = w.Write([]byte(`{"body":{"content":"text of ` + id + `"}}`))
		}
	}))
	defer server.Close()
	s := &Service{connections: ConnectionSettings{HTTPClient: server.Client(), AllowPrivate: true}}
	notices, err := s.bounceNotices(context.Background(), server.URL+"/me/sendMail", "synthetic-access", time.Date(2026, 11, 14, 9, 0, 0, 0, time.FixedZone("MYT", 8*3600)))
	require.NoError(t, err)
	assert.Equal(t, []bounceNotice{{Subject: "Undeliverable: Hi", Body: "text of m/2"}, {Subject: "Mail delivery failed", Body: "text of m3"}}, notices)
	assert.Equal(t, []string{"m/2", "m3"}, bodies, "only bounce notices are read; a link elsewhere is not followed")
}
