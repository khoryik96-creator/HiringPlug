package contacts

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// noDNS answers "no such host" for everything, as a machine without a
// working resolver does.
type noDNS struct{}

func (noDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (noDNS) LookupHost(_ context.Context, host string) ([]string, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestBrokenDNSIsUnknownNotInvalid(t *testing.T) {
	s := &Service{mail: noDNS{}}
	assert.Equal(t, MailServerFailed, s.lookupMailServer(context.Background(), "petronas.com"))
	check := s.checkEmail(context.Background(), ptr("hr@petronas.com"))
	assert.Equal(t, EmailUnknown, check.Status)
}
