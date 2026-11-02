package mail_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestSendingThroughSMTP(t *testing.T) {
	srv := testutil.NewSMTPServer(t)
	s := mail.NewSMTP(mail.Config{Addr: srv.Addr, From: "no-reply@hire.example", TLS: "none"})
	err := s.Send(context.Background(), mail.Message{
		FromName: "Aina (Contoh Recruitment)", ReplyTo: "aina@contoh.example",
		To: []string{"siti@example.com"}, Cc: []string{"boss@contoh.example"},
		Subject: "Interview on Tuesday – see you", Body: "Hi Siti,\nSee you on Tuesday.\n",
	})
	require.NoError(t, err)
	msgs := srv.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "no-reply@hire.example", msgs[0].From)
	assert.Equal(t, []string{"siti@example.com", "boss@contoh.example"}, msgs[0].To)
	data := msgs[0].Data
	assert.Contains(t, data, `From: "Aina (Contoh Recruitment)" <no-reply@hire.example>`)
	assert.Contains(t, data, "Reply-To: <aina@contoh.example>")
	assert.Contains(t, data, "Subject: =?utf-8?q?Interview_on_Tuesday_=E2=80=93_see_you?=")
	assert.Contains(t, data, "Hi Siti,\r\nSee you on Tuesday.")
}

func TestRefusalsAndInjection(t *testing.T) {
	srv := testutil.NewSMTPServer(t)
	s := mail.NewSMTP(mail.Config{Addr: srv.Addr, From: "no-reply@hire.example", TLS: "none"})
	err := s.Send(context.Background(), mail.Message{To: []string{"refuse@example.com"}, Subject: "x", Body: "x"})
	assert.True(t, errors.Is(err, mail.ErrPermanent), "a 5xx refusal is not retried: %v", err)

	_, err = mail.Build("no-reply@hire.example", mail.Message{To: []string{"a@example.com"}, Subject: "Hi\r\nBcc: victim@example.com", Body: "x"}, time.Now())
	assert.Error(t, err, "no headers can be added through the subject")

	err = mail.Unconfigured{}.Send(context.Background(), mail.Message{})
	assert.True(t, errors.Is(err, mail.ErrPermanent))
	assert.True(t, strings.Contains(err.Error(), "HP_SMTP_HOST"))
}

func TestStartTLSIsRequiredUnlessTurnedOff(t *testing.T) {
	srv := testutil.NewSMTPServer(t) // offers no STARTTLS
	err := mail.NewSMTP(mail.Config{Addr: srv.Addr, From: "no-reply@hire.example"}).Send(context.Background(),
		mail.Message{To: []string{"a@example.com"}, Subject: "x", Body: "x"})
	assert.True(t, errors.Is(err, mail.ErrPermanent), "never sends in the clear by surprise: %v", err)
}
