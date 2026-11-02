package contacts

// The email check: pure rules on an address and the result of looking up its
// domain's mail server. The lookup itself happens in the service.

import (
	"net/mail"
	"strings"
)

// Mail server lookup outcomes.
const (
	MailServerFound  = "found"  // the domain receives mail (MX, or an address record)
	MailServerNone   = "none"   // the domain does not exist or refuses mail
	MailServerFailed = "failed" // the lookup itself failed (timeout, network)
)

// Email check statuses, worst first.
const (
	EmailInvalid = "invalid"
	EmailUnknown = "unknown"
	EmailRisky   = "risky"
	EmailValid   = "valid"
)

// EmailDomain returns the part after @, lower-cased, or "" if there is none.
func EmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// WellFormedEmail reports whether email is a plain address (no display name)
// with a dotted domain.
func WellFormedEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && strings.Contains(EmailDomain(email), ".")
}

// CheckEmail classifies a well-formed or malformed address given the mail
// server lookup outcome (ignored for malformed addresses). Reasons say why, in
// words, so recruiters know what to do.
func CheckEmail(email, mailServer string) (string, []string) {
	if !WellFormedEmail(email) {
		return EmailInvalid, []string{"not a valid email address"}
	}
	switch mailServer {
	case MailServerNone:
		return EmailInvalid, []string{"the domain " + EmailDomain(email) + " does not receive email"}
	case MailServerFailed:
		return EmailUnknown, []string{"could not reach the domain's mail server; check again later"}
	}
	var reasons []string
	local := strings.ToLower(email[:strings.LastIndex(email, "@")])
	if roleMailboxes[local] {
		reasons = append(reasons, "a shared mailbox ("+local+"@), not a person")
	}
	if disposableDomains[EmailDomain(email)] {
		reasons = append(reasons, "a disposable address that stops working soon")
	}
	if len(reasons) > 0 {
		return EmailRisky, reasons
	}
	return EmailValid, nil
}

// Mailboxes shared by a team rather than one person.
var roleMailboxes = set(
	"admin", "career", "careers", "contact", "enquiry", "enquiries", "hello", "help", "hr", "humanresources",
	"info", "jobs", "mail", "marketing", "office", "recruit", "recruiter", "recruitment", "sales", "support",
	"talent", "team", "hiring", "noreply", "no-reply", "webmaster", "billing", "accounts",
)

// Well-known throwaway email services.
var disposableDomains = set(
	"mailinator.com", "guerrillamail.com", "guerrillamail.net", "10minutemail.com", "tempmail.com", "temp-mail.org",
	"yopmail.com", "trashmail.com", "getnada.com", "dispostable.com", "sharklasers.com", "maildrop.cc",
	"throwawaymail.com", "fakeinbox.com", "mintemail.com", "mohmal.com", "emailondeck.com", "tempail.com",
)

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}
