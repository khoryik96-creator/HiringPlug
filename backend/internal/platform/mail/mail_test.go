package mail_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	stdmail "net/mail"
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

func TestFormattedEmailsWithABannerAttachmentsAndBcc(t *testing.T) {
	srv := testutil.NewSMTPServer(t)
	s := mail.NewSMTP(mail.Config{Addr: srv.Addr, From: "no-reply@hire.example", TLS: "none"})
	err := s.Send(context.Background(), mail.Message{
		To: []string{"hr@client.example"}, Bcc: []string{"me@contoh.example"}, Subject: "Candidate: Siti Aisyah",
		Body: "Dear Ms Tan,\nPlease find Siti's CV.", HTML: `<p>Dear Ms Tan,</p><img src="cid:ebn_1"><p><b>Please</b> find Siti's CV.</p>`,
		Inline: []mail.Part{{Name: "banner.png", ContentType: "image/png", ContentID: "ebn_1", Data: []byte("PNGDATA")}},
		Attach: []mail.Part{{Name: "Siti Aisyah – CV.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7 cv")}},
	})
	require.NoError(t, err)
	msgs := srv.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, []string{"hr@client.example", "me@contoh.example"}, msgs[0].To, "bcc gets it")
	assert.NotContains(t, msgs[0].Data, "me@contoh.example", "but is not listed in it")

	msg, err := stdmail.ReadMessage(strings.NewReader(msgs[0].Data))
	require.NoError(t, err)
	mixed, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	assert.Equal(t, "multipart/mixed", mixed)
	r := multipart.NewReader(msg.Body, params["boundary"])
	first, err := r.NextPart()
	require.NoError(t, err)
	related, relParams, _ := mime.ParseMediaType(first.Header.Get("Content-Type"))
	assert.Equal(t, "multipart/related", related, "the formatted text with its banner")
	rr := multipart.NewReader(first, relParams["boundary"])
	alt, err := rr.NextPart()
	require.NoError(t, err)
	altType, altParams, _ := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	assert.Equal(t, "multipart/alternative", altType)
	ar := multipart.NewReader(alt, altParams["boundary"])
	text, _ := ar.NextPart()
	assert.Contains(t, text.Header.Get("Content-Type"), "text/plain")
	html, _ := ar.NextPart()
	htmlBody, _ := io.ReadAll(html) // multipart decodes quoted-printable
	assert.Contains(t, string(htmlBody), `<img src="cid:ebn_1">`)
	banner, err := rr.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "<ebn_1>", banner.Header.Get("Content-Id"))
	attached, err := r.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "Siti Aisyah – CV.pdf", attached.FileName())
	data, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, attached))
	assert.Equal(t, "%PDF-1.7 cv", string(data))

	_, err = mail.Build("no-reply@hire.example", mail.Message{To: []string{"a@example.com"}, Subject: "x", Body: "x",
		Attach: []mail.Part{{Name: "cv.pdf\r\nBcc: victim@example.com", ContentType: "application/pdf"}}}, time.Now())
	assert.Error(t, err, "no headers through a file name")
}

func TestACalendarInvitationIsAVersionOfTheMessageAndAnAttachment(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:x@hiringplug\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	raw, err := mail.Build("no-reply@hire.example", mail.Message{
		To: []string{"siti@example.com"}, Subject: "Call with Aina", Body: "Booked.\n", Calendar: ics,
	}, time.Now())
	require.NoError(t, err)
	msg, err := stdmail.ReadMessage(strings.NewReader(string(raw)))
	require.NoError(t, err)
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/mixed", mt)
	r := multipart.NewReader(msg.Body, params["boundary"])

	alt, err := r.NextPart()
	require.NoError(t, err)
	mt, altParams, err := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/alternative", mt)
	ar := multipart.NewReader(alt, altParams["boundary"])
	var kinds []string
	for {
		p, err := ar.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		kinds = append(kinds, p.Header.Get("Content-Type"))
		if strings.HasPrefix(p.Header.Get("Content-Type"), "text/calendar") {
			data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			require.NoError(t, err)
			assert.Equal(t, ics, string(data))
		}
	}
	assert.Equal(t, []string{`text/plain; charset="utf-8"`, `text/calendar; charset="utf-8"; method=REQUEST`}, kinds)

	att, err := r.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "application/ics; method=REQUEST", att.Header.Get("Content-Type"))
	assert.Equal(t, "invite.ics", att.FileName())

	_, err = mail.Build("no-reply@hire.example", mail.Message{To: []string{"a@b.c"}, Subject: "x", Body: "x", Calendar: "BEGIN:VCALENDAR\r\n"}, time.Now())
	assert.Error(t, err, "an invitation without a METHOD")
}
