package clientip

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOf(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:51234"
	r.Header.Set("X-Real-IP", "203.0.113.7")
	assert.Equal(t, "10.0.0.5", Of(r, ""), "headers are ignored unless configured: clients could fake them")
	assert.Equal(t, "203.0.113.7", Of(r, "X-Real-IP"), "the proxy's header when configured")
	r.Header.Del("X-Real-IP")
	assert.Equal(t, "10.0.0.5", Of(r, "X-Real-IP"))
}
