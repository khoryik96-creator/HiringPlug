package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// The access log never shows a secret in a path (an invitation or reset
// link, a booking or a shortlist link): whoever reads the logs could use it.
func TestTheAccessLogLeavesSecretsOutOfPaths(t *testing.T) {
	spec, err := api.GetSpec()
	require.NoError(t, err)
	paths := make([]string, 0, spec.Paths.Len())
	for p := range spec.Paths.Map() {
		paths = append(paths, p)
	}
	var log bytes.Buffer
	handler := observe(slog.New(slog.NewJSONHandler(&log, nil)), "", secretPathsOf(paths),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) }))
	secret := "3f9a0c1b2d4e5f60718293a4b5c6d7e8f9a0b1c2d3e4f5061728394a5b6c7d8e"
	for path, logged := range map[string]string{
		"/v1/auth/links/" + secret:                                    "/v1/auth/links/{linkToken}",
		"/v1/public/bookings/" + secret:                               "/v1/public/bookings/{bookingToken}",
		"/v1/public/client-shortlists/" + secret + "/candidates/x/cv": "/v1/public/client-shortlists/{shortlistToken}/candidates/x/cv",
		"/v1/auth/links/" + secret + "/no-such-route":                 "/v1/auth/links/{linkToken}/no-such-route",
		"/v1/candidates/cand_1":                                       "/v1/candidates/cand_1", // ids stay, for finding a request
	} {
		log.Reset()
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
		}
		assert.NotContains(t, log.String(), secret, path)
		assert.Contains(t, log.String(), `"path":"`+logged+`"`, path)
	}
}
