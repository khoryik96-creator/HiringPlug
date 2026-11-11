package mail_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Compress the network deadline so a stalled server test takes milliseconds
// instead of waiting for the production timeout. No deadline still hangs.
type shortDeadlineConn struct{ net.Conn }

func (c shortDeadlineConn) SetDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		deadline = minTime(deadline, time.Now().Add(25*time.Millisecond))
	}
	return c.Conn.SetDeadline(deadline)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func TestImplicitTLSHandshakeCannotStallDeliveryForever(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close(); _ = server.Close() }()
	go func() { _, _ = io.Copy(io.Discard, server) }() // consume ClientHello, never answer
	sender := mail.NewSMTP(mail.Config{Addr: "smtp.example.test:465", From: "sender@example.test", TLS: "tls",
		DialContext: func(context.Context, string, string) (net.Conn, error) { return shortDeadlineConn{client}, nil },
	})
	done := make(chan error, 1)
	go func() {
		done <- sender.Send(context.Background(), mail.Message{To: []string{"receiver@example.test"}, Subject: "Timeout test", Body: "Synthetic"})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a silent TLS server must fail delivery")
		}
	case <-time.After(time.Second):
		t.Fatal("implicit TLS handshake has no network deadline")
	}
}
