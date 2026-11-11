package emails

import (
	"fmt"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailboxSecretsAndOAuthStateAreBoundToTheirOwner(t *testing.T) {
	service := &Service{connections: ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-secret-key-at-least-32-characters")}}
	sealed, err := service.sealMailbox("tenant-a", "user-a", "google", mailboxSecret{Password: "synthetic-password"})
	require.NoError(t, err)
	assert.NotContains(t, sealed, "synthetic-password")
	var opened mailboxSecret
	require.NoError(t, service.openMailbox("tenant-a", "user-a", "google", sealed, &opened))
	assert.Equal(t, "synthetic-password", opened.Password)
	for _, identity := range [][3]string{{"tenant-b", "user-a", "google"}, {"tenant-a", "user-b", "google"}, {"tenant-a", "user-a", "smtp"}} {
		assert.Error(t, service.openMailbox(identity[0], identity[1], identity[2], sealed, &opened))
	}
	state := service.signMailboxState(oauthState{Tenant: "tenant-a", User: "user-a", Provider: "google", Nonce: "synthetic-nonce", Expires: time.Now().Add(time.Minute).Unix()})
	_, err = service.readMailboxState(state)
	require.NoError(t, err)
	parts := strings.Split(state, ".")
	parts[0] = "tampered"
	_, err = service.readMailboxState(strings.Join(parts, "."))
	assert.Error(t, err)
	expired := service.signMailboxState(oauthState{Tenant: "tenant-a", User: "user-a", Provider: "google", Expires: time.Now().Add(-time.Minute).Unix()})
	_, err = service.readMailboxState(expired)
	assert.Error(t, err)
}

func TestSMTPReplyTextCannotLeakSecretsIntoDeliveryErrors(t *testing.T) {
	err := mailboxSMTPError(fmt.Errorf("%w: %w", mail.ErrPermanent, &textproto.Error{Code: 535, Msg: "synthetic-password and private salary text"}))
	assert.ErrorIs(t, err, mail.ErrPermanent)
	assert.Contains(t, err.Error(), "535")
	assert.NotContains(t, err.Error(), "synthetic-password")
	assert.NotContains(t, err.Error(), "private salary")
}
