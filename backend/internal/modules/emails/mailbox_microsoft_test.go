package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

func TestLargeMicrosoftAttachmentsUseDraftAndAuthorizedChunks(t *testing.T) {
	var server *httptest.Server
	var uploaded, sent int
	payload := bytes.Repeat([]byte("x"), 4*1024*1024+123)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/me/messages":
			assert.Equal(t, "Bearer synthetic-access", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"id":"synthetic-draft"}`))
		case "/me/messages/synthetic-draft/attachments/createUploadSession":
			var body map[string]map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.EqualValues(t, len(payload), body["AttachmentItem"]["size"])
			_, _ = w.Write([]byte(`{"uploadUrl":"` + server.URL + `/upload?synthetic-preauthorized=1"}`))
		case "/upload":
			assert.Empty(t, r.Header.Get("Authorization"), "the upload URL must not receive the OAuth token")
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(data), 10*320*1024)
			assert.True(t, strings.HasPrefix(r.Header.Get("Content-Range"), "bytes "+strconv.Itoa(uploaded)+"-"))
			assert.Equal(t, payload[uploaded:uploaded+len(data)], data)
			uploaded += len(data)
			w.WriteHeader(201)
		case "/me/messages/synthetic-draft/send":
			sent++
			w.WriteHeader(202)
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	service := &Service{connections: ConnectionSettings{HTTPClient: server.Client(), AllowPrivate: true}}
	err := service.sendLargeMicrosoftMail(context.Background(), OAuthProvider{SendURL: server.URL + "/me/sendMail"}, "synthetic-access", mail.Message{Subject: "Synthetic CV", Body: "CV attached", To: []string{"receiver@example.test"}, Attach: []mail.Part{{Name: "Synthetic CV.pdf", ContentType: "application/pdf", Data: payload}}})
	require.NoError(t, err)
	assert.Equal(t, len(payload), uploaded)
	assert.Equal(t, 1, sent)
	service.connections.AllowPrivate = false
	assert.False(t, service.trustedUploadURL("http://127.0.0.1/private", "https://graph.microsoft.com/v1.0/me/sendMail"))
	assert.False(t, service.trustedUploadURL("https://outlook.office.com.attacker.test/upload", "https://graph.microsoft.com/v1.0/me/sendMail"))
}
