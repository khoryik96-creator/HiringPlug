package safehttp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlocked(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.10", "172.16.5.4", "169.254.169.254", "::1", "fd00::1", "0.0.0.0", "100.64.0.1", "::ffff:127.0.0.1"} {
		assert.True(t, Blocked(net.ParseIP(ip), false), ip)
	}
	assert.False(t, Blocked(net.ParseIP("93.184.216.34"), false))
	assert.False(t, Blocked(net.ParseIP("127.0.0.1"), true), "allowed for local testing")
}

func TestClientRefusesLocalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	res, err := Client(Options{Timeout: time.Second}).Get(srv.URL) //nolint:noctx // test
	if res != nil {
		_ = res.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private or local network address")
	res, err = Client(Options{AllowPrivate: true, Timeout: time.Second}).Get(srv.URL) //nolint:noctx // test
	require.NoError(t, err)
	_ = res.Body.Close()
}
