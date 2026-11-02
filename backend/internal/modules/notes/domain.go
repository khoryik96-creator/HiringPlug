package notes

// The notes rules, as plain functions on plain values: no database, no HTTP.

import (
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Note is one note on a record.
type Note struct {
	ID          string
	SubjectType string
	SubjectID   string
	Kind        string
	Body        string
	Sensitive   bool
	AuthorID    string
	AuthorName  string
	CreatedAt   time.Time
}

// Draft is a note being written.
type Draft struct {
	SubjectType string
	SubjectID   string
	Kind        string
	Body        string
	Sensitive   bool
}

// Normalize trims the note and fills in the default kind. canSensitive says
// whether the author may read (and so write) sensitive notes.
func Normalize(d Draft, canSensitive bool) (Draft, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.Body = strings.TrimSpace(d.Body)
	if d.Body == "" {
		errs.Add("body", "is required")
	}
	if d.Kind == "" {
		d.Kind = "general"
	}
	if d.Sensitive && !canSensitive {
		errs.Add("sensitive", "you may not write sensitive notes (they need notes.sensitive:read)")
	}
	return d, errs
}

// MayDelete reports whether a user may delete a note: their own, or any with
// notes:manage.
func MayDelete(n Note, userID string, canManage bool) bool {
	return canManage || n.AuthorID == userID
}

// Activity is one entry of a record's history.
type Activity struct {
	ID        string
	Action    string
	ActorName string
	Changes   map[string]any
	At        time.Time
}
