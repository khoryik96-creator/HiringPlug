// Package mailcheck checks whether an email address's mailbox exists, by
// asking its mail server (a Go port of CV Studio's email verifier). It never
// sends an email: one SMTP session asks the server whether it takes mail for
// the address and for a made-up address beside it (to spot servers that take
// any address, "catch-all"), then hangs up.
//
// The answers are evidence, not proof: "accepted" does not guarantee
// delivery, "catch-all" is a caution not a verdict, a policy refusal is not a
// missing mailbox, and any network or DNS failure is "unknown", never
// "invalid". Only public addresses are contacted, on port 25 only, within a
// 25-second budget per check; at most two checks run at a time per process,
// and one per domain.
package mailcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Statuses.
const (
	Accepted = "accepted"  // the server took the mailbox and refused a made-up one
	CatchAll = "catch_all" // the server takes any address: the mailbox is not established
	Invalid  = "invalid"   // the address cannot receive mail (bad format, no domain, no mailbox)
	Unknown  = "unknown"   // no conclusion either way
)

// Modes.
const (
	Full = "full" // up to two sessions, stopping at the first clear answer
	Deep = "deep" // compares the domain's mail servers: up to four sessions
)

// Budget is the time one check may take, DNS and all sessions together.
const Budget = 25 * time.Second

// RetryAfter is when a retryable "unknown" is worth checking again.
const RetryAfter = 2 * time.Minute

// ControlPrefix starts the made-up mailbox's local part.
const ControlPrefix = "hiringplug-"

var controlRe = regexp.MustCompile(`^[a-f0-9]{32}$`)

// NewControlID returns a fresh made-up mailbox identity: keep it for the
// address's rechecks, so every session asks about the same one.
func NewControlID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Result is what a check established.
type Result struct {
	Status    string
	Reason    string
	Outcome   string // recipient_supported, catch_all, recipient_missing, policy, sender_policy, temporary, mailbox_full, tls, timeout, connection, dns_unavailable, check_unavailable, conflicting_servers, no_domain, no_mail, no_server, bad_format, unsupported
	Syntax    string
	Domain    string // mx, implicit, no_domain, no_mail, no_server, not_checked
	Mailbox   string // the target mailbox: accepted, rejected, unconfirmed, conflicting, not_checked
	Provider  string
	Attempts  int
	Retryable bool
}

// Verifier runs checks.
type Verifier struct {
	DNS  Resolver
	Dial Dialer

	slots   chan struct{}
	mu      sync.Mutex
	domains map[string]chan struct{}
}

// New returns a verifier using the system DNS and public port-25 connections.
func New() *Verifier {
	return &Verifier{DNS: net.DefaultResolver, Dial: PublicDialer()}
}

func (v *Verifier) init() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.slots == nil {
		v.slots = make(chan struct{}, 2)
		v.domains = map[string]chan struct{}{}
	}
}

// lockDomain waits until no other check of domain runs (or ctx ends).
func (v *Verifier) lockDomain(ctx context.Context, domain string) (func(), error) {
	for {
		v.mu.Lock()
		busy, taken := v.domains[domain]
		if !taken {
			ch := make(chan struct{})
			v.domains[domain] = ch
			v.mu.Unlock()
			return func() {
				v.mu.Lock()
				delete(v.domains, domain)
				v.mu.Unlock()
				close(ch)
			}, nil
		}
		v.mu.Unlock()
		select {
		case <-busy:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Check checks email's mailbox. controlID (from NewControlID) names the
// made-up mailbox; reuse it when checking the same address again.
func (v *Verifier) Check(ctx context.Context, email, mode, controlID string) (Result, error) {
	v.init()
	address, domain, syntax := AddressParts(email)
	r := Result{Status: Unknown, Syntax: syntax, Domain: DomainNotChecked, Mailbox: "not_checked", Provider: "Not checked"}
	switch syntax {
	case SyntaxInvalid:
		r.Status, r.Outcome, r.Reason = Invalid, "bad_format", "This address has an invalid format. Check its spelling and punctuation."
		return r, nil
	case SyntaxUnsupported:
		r.Outcome, r.Reason = "unsupported", "Quoted mailbox names, international mailbox names and address literals cannot be checked. This address has not been marked invalid."
		return r, nil
	}
	if !controlRe.MatchString(controlID) {
		controlID = NewControlID()
	}
	select {
	case v.slots <- struct{}{}:
	case <-ctx.Done():
		return r, ctx.Err()
	}
	defer func() { <-v.slots }()
	unlock, err := v.lockDomain(ctx, domain)
	if err != nil {
		return r, err
	}
	defer unlock()

	deadline := time.Now().Add(Budget)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	unavailableResult := func(err error) Result {
		u, ok := err.(*unavailable)
		if !ok {
			u = errDNSFailed
		}
		r.Status, r.Reason, r.Retryable = Unknown, u.reason, u.transient
		r.Outcome = "check_unavailable"
		if u.transient {
			r.Outcome = "dns_unavailable"
		}
		return r
	}
	state, hostNames, err := hosts(ctx, v.DNS, domain, deadline)
	if err != nil {
		return unavailableResult(err), nil
	}
	r.Domain = state
	if state == DomainNoMail {
		r.Status, r.Outcome, r.Reason = Invalid, "no_mail", "The domain explicitly declares that it does not receive email."
		return r, nil
	}
	type target struct{ host, ip string }
	var reachable []target
	var byHost [][]target
	var lookupFailure error
	for _, host := range hostNames {
		ips, err := addresses(ctx, v.DNS, host, deadline)
		if err != nil {
			if lookupFailure == nil {
				lookupFailure = err
			}
			continue // a failed server must not hide another usable one
		}
		var row []target
		for _, ip := range ips {
			row = append(row, target{host, ip})
		}
		reachable = append(reachable, row...)
		byHost = append(byHost, row)
	}
	if len(reachable) == 0 {
		if lookupFailure != nil {
			return unavailableResult(lookupFailure), nil
		}
		r.Domain = DomainNoServer
		if state == DomainImplicit {
			r.Status, r.Outcome, r.Reason = Invalid, "no_domain", "The domain does not exist or has no mail server."
		} else {
			r.Outcome, r.Reason = "no_server", "The advertised mail server has no usable address. Mailbox validity is unknown."
		}
		return r, nil
	}
	plan := reachable[:min(2, len(reachable))]
	if mode == Deep { // one address of each server first, then the second ones
		plan = nil
		for i := range 2 {
			for _, row := range byHost {
				if len(row) > i {
					plan = append(plan, row[i])
				}
			}
		}
		plan = plan[:min(4, len(plan))]
	}
	controlLocal := ControlPrefix + controlID
	var seen []observation
	settled := map[string]bool{}
	for len(plan) > 0 && time.Now().Before(deadline) {
		t := plan[0]
		plan = plan[1:]
		if mode == Deep && settled[t.host] {
			continue
		}
		o := probe(ctx, v.Dial, t.host, t.ip, address, controlLocal, deadline)
		r.Attempts++
		r.Provider = Provider(t.host)
		seen = append(seen, o)
		r.Status, r.Reason, r.Outcome = o.Status, o.Reason, o.Outcome
		if o.Status != Unknown {
			settled[t.host] = true
			if mode != Deep {
				break
			}
		} else if mode == Deep && o.Outcome == "connection" {
			// A blocked IPv4 route: try this server's other address family next.
			v6 := strings.Contains(t.ip, ":")
			for i, next := range plan {
				if next.host == t.host && strings.Contains(next.ip, ":") != v6 {
					plan = append([]target{next}, append(plan[:i:i], plan[i+1:]...)...)
					break
				}
			}
		}
	}
	if len(seen) == 0 {
		r.Outcome, r.Retryable = "timeout", true
		r.Reason = "The check reached its time limit before a mailbox response. The address remains unconfirmed."
		return r, nil
	}
	accepted, rejected := false, false
	for _, o := range seen {
		accepted = accepted || o.Target == "accepted"
		rejected = rejected || o.Target == "rejected"
	}
	switch {
	case accepted && rejected:
		r.Status, r.Outcome = Unknown, "conflicting_servers"
		r.Reason = "Mail servers gave conflicting answers about this mailbox. It needs more evidence; no conclusion was made."
	case mode == Deep:
		pick := seen[len(seen)-1]
		for _, want := range []string{CatchAll, Accepted, Invalid} {
			if i := indexStatus(seen, want); i >= 0 {
				pick = seen[i]
				break
			}
		}
		r.Status, r.Reason, r.Outcome = pick.Status, pick.Reason, pick.Outcome
	}
	switch {
	case r.Outcome == "conflicting_servers":
		r.Mailbox = "conflicting"
	case r.Status == Invalid:
		r.Mailbox = "rejected"
	case r.Status == Accepted || r.Status == CatchAll || accepted:
		r.Mailbox = "accepted"
	default:
		r.Mailbox = "unconfirmed"
	}
	if r.Status == Unknown {
		blocked := false
		for _, o := range seen {
			blocked = blocked || o.Outcome == "policy" || o.Outcome == "sender_policy" || o.Outcome == "tls" || o.Outcome == "mailbox_full"
			r.Retryable = r.Retryable || o.Retryable
		}
		r.Retryable = r.Retryable && !blocked && r.Outcome != "conflicting_servers"
	}
	return r, nil
}

func indexStatus(seen []observation, status string) int {
	for i, o := range seen {
		if o.Status == status {
			return i
		}
	}
	return -1
}
