// Package salaryview names the moments a candidate's salary is shown to a
// person, for the log of who was shown a salary (the auditlog module keeps
// it). A module calls In where salary really leaves the server for a
// person: in a response they see, a file they download or an email they
// send. The recorder is passed in from internal/app (modules do not import
// each other); it never stores an amount, and never counts the system.
package salaryview

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Kinds: where a salary was shown.
const (
	Candidate     = "candidate"      // a candidate's page
	CandidateList = "candidate_list" // a list or search with salary columns (one line for all of them)
	Export        = "export"         // a file of candidates or placements with salary (one line for all)
	EmailFields   = "email_fields"   // an email being written, its fields filled with the salary
	EmailSent     = "email_sent"     // an email sent with the salary
	CV            = "cv"             // a formatted CV with the salary: made, opened, downloaded or attached
	Screening     = "screening"      // a screening call's salary
	Offer         = "offer"          // an offer's salary
	Placement     = "placement"      // a placement's salary
)

// Kinds are every kind, in the order they are offered.
var Kinds = []string{Candidate, CandidateList, Export, EmailFields, EmailSent, CV, Screening, Offer, Placement}

// Grouped kinds are one line a day for many candidates; the others one line
// per candidate.
func Grouped(kind string) bool { return kind == CandidateList || kind == Export }

// Recorder counts a showing in the caller's transaction, as the
// transaction's user (nothing for the system). For a grouped kind,
// candidateIDs are every candidate whose salary it showed; otherwise one
// line is counted for each.
type Recorder interface {
	SalarySeen(ctx context.Context, tx pgx.Tx, kind string, candidateIDs ...string) error
}

// In counts a showing in tx: nothing without a recorder or a candidate.
func In(ctx context.Context, tx pgx.Tx, r Recorder, kind string, candidateIDs ...string) error {
	if r == nil || len(candidateIDs) == 0 {
		return nil
	}
	return r.SalarySeen(ctx, tx, kind, candidateIDs...)
}
