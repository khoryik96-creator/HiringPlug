package shortlists

// The rules for client shortlist links, as plain functions on plain values:
// what a link holds, when it works, what the client may answer, and the
// email that carries it.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Limits of a link.
const (
	DefaultDays   = 14 // how long a link works unless the recruiter chooses
	MaxDays       = 90
	MaxCandidates = 20
	maxNote       = 1000
	maxMessage    = 2000
	maxComment    = 2000
)

// A link's status.
const (
	Open    = "open"
	Expired = "expired"
	Revoked = "revoked"
)

// The client's answers.
const (
	Approved = "approved"
	Rejected = "rejected"
)

// Shortlist is a link as the recruiter sees it.
type Shortlist struct {
	ID, JobID, Status       string
	Message, EmailedTo      *string
	CreatedByName           *string
	ExpiresAt, CreatedAt    time.Time
	RevokedAt, OpenedAt     *time.Time
	LastOpenedAt            *time.Time
	Candidates              []Candidate
}

// Candidate is one candidate on a link, with the client's answer.
type Candidate struct {
	ID, ApplicationID, CandidateID string
	CandidateName                  string // the real name, for the recruiter
	Label                          string // the name the client sees
	DocumentID                     string
	Blind                          bool
	Note, Decision, Comment        *string
	AnsweredAt                     *time.Time
}

// Pick is a candidate chosen for a link, with their CV.
type Pick struct {
	ApplicationID, DocumentID string
	Note                      *string
}

// Input is a new link.
type Input struct {
	Picks              []Pick
	ExpiresInDays      *int
	Message            *string
	EmailHiringContact bool
}

// NormalizeInput tidies and checks a new link; it returns how many days it works.
func NormalizeInput(in Input) (Input, int, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	switch n := len(in.Picks); {
	case n == 0:
		errs.Add("candidates", "choose at least one candidate")
	case n > MaxCandidates:
		errs.Add("candidates", fmt.Sprintf("at most %d candidates on one link", MaxCandidates))
	}
	seen := map[string]bool{}
	for i, p := range in.Picks {
		if seen[p.ApplicationID] {
			errs.Add(fmt.Sprintf("candidates.%d.applicationId", i), "lists a candidate twice")
		}
		seen[p.ApplicationID] = true
		in.Picks[i].Note = Tidy(p.Note)
		if in.Picks[i].Note != nil && len([]rune(*in.Picks[i].Note)) > maxNote {
			errs.Add(fmt.Sprintf("candidates.%d.note", i), fmt.Sprintf("at most %d characters", maxNote))
		}
	}
	days := DefaultDays
	if in.ExpiresInDays != nil {
		days = *in.ExpiresInDays
	}
	if days < 1 || days > MaxDays {
		errs.Add("expiresInDays", fmt.Sprintf("between 1 and %d days", MaxDays))
	}
	in.Message = Tidy(in.Message)
	if in.Message != nil && len([]rune(*in.Message)) > maxMessage {
		errs.Add("message", fmt.Sprintf("at most %d characters", maxMessage))
	}
	return in, days, errs
}

// Tidy trims text; blank is nil.
func Tidy(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// Status is a link's status at a time.
func Status(expiresAt time.Time, revokedAt *time.Time, now time.Time) string {
	switch {
	case revokedAt != nil:
		return Revoked
	case !now.Before(expiresAt):
		return Expired
	}
	return Open
}

// NormalizeAnswer checks the client's answer and tidies the comment.
func NormalizeAnswer(decision string, comment *string) (*string, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if decision != Approved && decision != Rejected {
		errs.Add("decision", "must be approved or rejected")
	}
	comment = Tidy(comment)
	if comment != nil && len([]rune(*comment)) > maxComment {
		errs.Add("comment", fmt.Sprintf("at most %d characters", maxComment))
	}
	return comment, errs
}

// Label is the name the client sees: the reference with a blind CV, else the
// candidate's name.
func Label(cv ClientCV, name string) string {
	if cv.Blind || strings.TrimSpace(name) == "" {
		return cv.Reference
	}
	return strings.TrimSpace(name)
}

// CVFileName is the name the client's download gets: the label, never the
// stored file's name (which may name the candidate).
func CVFileName(label, stored string) string {
	ext := strings.ToLower(filepath.Ext(stored))
	if ext != ".pdf" && ext != ".docx" && ext != ".doc" {
		ext = ".pdf"
	}
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, label)
	return name + " CV" + ext
}

// NewToken is a link's secret: 32 random bytes, URL-safe. Only its hash is stored.
func NewToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is what is stored of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LinkEmail is the email that takes the link to the hiring contact.
func LinkEmail(contact, jobTitle, url string, n int, message *string, expires time.Time, sender, agency string) (subject, body string) {
	subject = fmt.Sprintf("Shortlist for %s: %d candidate%s", jobTitle, n, plural(n))
	greeting := "Hello,"
	if c := strings.TrimSpace(contact); c != "" {
		greeting = "Hello " + strings.Fields(c)[0] + ","
	}
	var b strings.Builder
	b.WriteString(greeting + "\n\n")
	if message != nil {
		b.WriteString(*message + "\n\n")
	}
	fmt.Fprintf(&b, "Here is the shortlist for %s (%d candidate%s):\n%s\n\n", jobTitle, n, plural(n), url)
	b.WriteString("Open the link to read each CV, then approve or reject each candidate, with a comment if you like. ")
	fmt.Fprintf(&b, "The link works until %s.\n\n", expires.Format("2 January 2006"))
	b.WriteString("Best regards,\n" + sender)
	if agency != "" {
		b.WriteString("\n" + agency)
	}
	return subject, b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
