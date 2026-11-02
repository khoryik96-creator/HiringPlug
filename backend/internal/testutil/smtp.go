package testutil

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
)

// SMTPServer is a tiny SMTP server for tests: it accepts every message,
// except recipients whose address starts with "refuse" (refused for good,
// 550) or "busy" (try again later, 451), and keeps them.
type SMTPServer struct {
	Addr string
	ln   net.Listener
	mu   sync.Mutex
	msgs []SMTPMessage
}

// SMTPMessage is one received message.
type SMTPMessage struct {
	From string
	To   []string
	Data string
}

// NewSMTPServer starts a server on a free local port, stopped with the test.
func NewSMTPServer(t *testing.T) *SMTPServer {
	t.Helper()
	s, err := StartSMTPServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// StartSMTPServer starts a server on a free local port (for a whole test
// package; stop it with Close).
func StartSMTPServer() (*SMTPServer, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &SMTPServer{Addr: ln.Addr().String(), ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s, nil
}

// Close stops the server.
func (s *SMTPServer) Close() { _ = s.ln.Close() }

// To returns the messages received for an address.
func (s *SMTPServer) To(address string) []SMTPMessage {
	var out []SMTPMessage
	for _, m := range s.Messages() {
		for _, to := range m.To {
			if to == address {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// Messages returns what was received so far.
func (s *SMTPServer) Messages() []SMTPMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SMTPMessage(nil), s.msgs...)
}

func (s *SMTPServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r, w := bufio.NewReader(conn), bufio.NewWriter(conn)
	reply := func(line string) { _, _ = w.WriteString(line + "\r\n"); _ = w.Flush() }
	reply("220 test ESMTP")
	var msg SMTPMessage
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 test")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			msg = SMTPMessage{From: addr(line)}
			reply("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if strings.HasPrefix(addr(line), "refuse") {
				reply("550 no such user")
				continue
			}
			if strings.HasPrefix(addr(line), "busy") {
				reply("451 try again later")
				continue
			}
			msg.To = append(msg.To, addr(line))
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go ahead")
			var data strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(l)
			}
			msg.Data = data.String()
			s.mu.Lock()
			s.msgs = append(s.msgs, msg)
			s.mu.Unlock()
			reply("250 queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func addr(line string) string {
	start, end := strings.IndexByte(line, '<'), strings.IndexByte(line, '>')
	if start < 0 || end < start {
		return ""
	}
	return line[start+1 : end]
}
