package mail

import (
	"net"
	"strings"
)

// DeliveryMode identifies the built-in local catcher; provider acceptance is
// never a guarantee of arrival in the recipient's inbox.
func DeliveryMode(address string) string {
	if address == "" {
		return "unconfigured"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "live"
	}
	if strings.EqualFold(host, "mailpit") || port == "1025" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return "test"
	}
	return "live"
}
