package emails

// Real bounces: after an email goes through a connected Microsoft 365
// mailbox (which HiringPlug may read), its inbox is looked at twice for
// bounce notices ("Undeliverable: ...") naming one of the email's
// recipients. Those recipients are kept on the email, and their contact's
// mailbox is marked as bouncing. The notices themselves are not kept.

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// BounceScans are when, after sending, the mailbox is looked at for notices.
var BounceScans = []time.Duration{15 * time.Minute, 2 * time.Hour}

// The subjects mail servers give bounce notices (Exchange/Microsoft 365,
// Gmail, Postfix, Exim, qmail ...).
var bounceSubject = regexp.MustCompile(`(?i)^\s*(undeliverable|undelivered mail returned to sender|delivery status notification \(failure\)|mail delivery failed|returned mail|delivery has failed|failure notice|delivery failure|message not delivered)(\W|$)`)

// tags are HTML tags, except an address in angle brackets ("<ana@x.com>" in
// a plain-text notice); a tag naming one (a mailto: link) is kept, harmlessly.
var tags = regexp.MustCompile(`(?s)<[^>@]*>`)

// ProviderMicrosoft is the mailbox provider whose inbox HiringPlug may read
// (Mail.ReadWrite); Gmail and SMTP mailboxes are connected to send only.
const ProviderMicrosoft = "microsoft"

// quoted is where a notice starts quoting the email it returns (its headers
// name every recipient, not only those that bounced): the text after it is
// not read.
var quoted = regexp.MustCompile(`(?im)original message headers|-{2,}\s*original message|this is a copy of the message|message/rfc822|^\s*(to|cc|from|received|subject|date|message-id|return-path):`)

// BouncedRecipients returns which of recipients a notice says could not be
// reached: only for a bounce notice's subject, and only addresses it names
// exactly (any letter case) before it quotes the returned email.
func BouncedRecipients(subject, body string, recipients []string) []string {
	if !bounceSubject.MatchString(subject) {
		return nil
	}
	text := strings.ToLower(html.UnescapeString(tags.ReplaceAllString(body, "\n")))
	if at := quoted.FindStringIndex(text); at != nil {
		text = text[:at[0]]
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range recipients {
		addr := strings.ToLower(strings.TrimSpace(r))
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		if containsAddress(text, addr) {
			out = append(out, addr)
		}
	}
	return out
}

// containsAddress reports whether text has addr as a whole address, not part
// of a longer one ("ana@x.com" in "joana@x.com" or "ana@x.com.my" is not; a
// full stop ending a sentence is not part of it).
func containsAddress(text, addr string) bool {
	charAt := func(i int) byte {
		if i < 0 || i >= len(text) {
			return ' '
		}
		return text[i]
	}
	for start := 0; ; {
		i := strings.Index(text[start:], addr)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(addr)
		after := charAt(end)
		if after == '.' {
			after = charAt(end + 1)
		}
		if !addressChar(charAt(i-1)) && !addressChar(after) {
			return true
		}
		start = i + 1
	}
}

func addressChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.IndexByte("._%+-@", b) >= 0
}

// Reading the inbox: subjects first (small), up to inboxPages pages of
// inboxPage; then the text of at most maxNotices that look like bounces.
const (
	inboxPage  = 100
	inboxPages = 5
	maxNotices = 20
)

// bounceNotice is a bounce notice's subject and text.
type bounceNotice struct{ Subject, Body string }

// bounceNotices reads, from a Microsoft 365 inbox, the messages received
// since a moment whose subject looks like a bounce notice.
func (s *Service) bounceNotices(ctx context.Context, sendURL, token string, since time.Time) ([]bounceNotice, error) {
	root := strings.TrimSuffix(sendURL, "/sendMail")
	inbox := root + "/mailFolders/inbox/messages"
	q := url.Values{}
	q.Set("$filter", "receivedDateTime ge "+since.UTC().Format(time.RFC3339))
	q.Set("$select", "id,subject")
	q.Set("$top", strconv.Itoa(inboxPage))
	var ids []string
	var subjects []string
	for page, next := 0, inbox+"?"+q.Encode(); next != "" && page < inboxPages && len(ids) < maxNotices; page++ {
		raw, err := s.providerRequest(ctx, "GET", next, "application/json", token, nil)
		if err != nil {
			return nil, err
		}
		var list struct {
			Value []struct {
				ID      string `json:"id"`
				Subject string `json:"subject"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
		for _, m := range list.Value {
			if m.ID != "" && bounceSubject.MatchString(m.Subject) && len(ids) < maxNotices {
				ids, subjects = append(ids, m.ID), append(subjects, m.Subject)
			}
		}
		next = ""
		if strings.HasPrefix(list.Next, inbox+"?") { // only the same folder: the token goes with it
			next = list.Next
		}
	}
	out := make([]bounceNotice, 0, len(ids))
	for i, id := range ids {
		raw, err := s.providerRequest(ctx, "GET", root+"/messages/"+url.PathEscape(id)+"?%24select=body", "application/json", token, nil)
		if err != nil {
			return nil, err
		}
		var m struct {
			Body struct {
				Content string `json:"content"`
			} `json:"body"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, bounceNotice{Subject: subjects[i], Body: m.Body.Content})
	}
	return out, nil
}

// readableMailbox returns the mailbox an email went through, if HiringPlug
// may read its inbox and it is still the sender's.
func readableMailbox(ctx context.Context, q *store.Queries, e store.Email) (store.EmailMailbox, bool, error) {
	if e.MailboxRef == nil {
		return store.EmailMailbox{}, false, nil
	}
	row, err := q.MailboxForDelivery(ctx, e.SentBy)
	if database.IsNoRows(err) {
		return row, false, nil
	}
	if err != nil {
		return row, false, err
	}
	return row, row.ID == *e.MailboxRef && row.Provider == ProviderMicrosoft, nil // not disconnected since
}

// scheduleBounceScans queues the looks for bounce notices of an email sent
// through a Microsoft 365 mailbox.
func scheduleBounceScans(ctx context.Context, tx pgx.Tx, tenantID, emailID string) error {
	for _, after := range BounceScans {
		if _, err := jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindEmailBounceScan, api.EmailBounceScanJob{EmailId: emailID}, time.Now().Add(after)); err != nil {
			return err
		}
	}
	return nil
}

// scanBounces handles email.bounce_scan.
func (s *Service) scanBounces(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.EmailBounceScanJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	e, err := q.GetEmail(ctx, payload.EmailId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if e.Status != string(api.EmailStatusSent) || e.SentAt == nil {
		return nil
	}
	row, ok, err := readableMailbox(ctx, q, e)
	if !ok || err != nil {
		return err // disconnected since, or a mailbox HiringPlug cannot read
	}
	token, err := s.mailboxAccess(ctx, row)
	if errors.Is(err, mail.ErrPermanent) { // to be connected again: nothing to retry here
		logx.From(ctx).Info("bounce scan skipped: mailbox not usable", "email_id", e.ID)
		return nil
	}
	if err != nil {
		return err // e.g. the provider was briefly unavailable: the queue tries again
	}
	notices, err := s.bounceNotices(ctx, s.endpoint(row.Provider).SendURL, token, e.SentAt.Add(-time.Minute))
	if err != nil {
		return err
	}
	recipients := append(append(append([]string{}, e.ToAddresses...), e.CcAddresses...), e.BccAddresses...)
	bounced := map[string]bool{}
	for _, b := range e.BouncedTo {
		bounced[b] = true
	}
	var found []string
	for _, m := range notices {
		for _, addr := range BouncedRecipients(m.Subject, m.Body, recipients) {
			if !bounced[addr] {
				bounced[addr] = true
				found = append(found, addr)
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	all := append(append([]string{}, e.BouncedTo...), found...)
	if err := q.MarkBounced(ctx, store.MarkBouncedParams{ID: e.ID, BouncedTo: all}); err != nil {
		return err
	}
	if s.deps.Bounces != nil {
		for _, addr := range found {
			if err := s.deps.Bounces.Bounced(ctx, tx, addr); err != nil {
				return err
			}
		}
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "email.bounced", EntityType: "email", EntityID: e.ID, Changes: map[string]any{"bouncedTo": found}})
}
