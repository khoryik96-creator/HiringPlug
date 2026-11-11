package mail

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalCatcherIsDistinctFromLiveSMTP(t *testing.T) {
	for address, mode := range map[string]string{"": "unconfigured", "mailpit:1025": "test", "localhost:1025": "test", "[::1]:1025": "test", "smtp.example.test:587": "live", "smtp.example.test:1025": "live"} {
		assert.Equal(t, mode, DeliveryMode(address))
	}
}
