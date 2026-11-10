package intake

// Statuses of an imported file.
const (
	Reading = "reading" // being read
	Created = "created" // a new candidate, filled in from the file
	Added   = "added"   // the file went to the candidate who already has its email
	Unread  = "unread"  // a new candidate named after the file; its details could not be read
	Failed  = "failed"  // not a CV file
	// Duplicate: exactly a CV already in HiringPlug; skipped, no candidate made.
	Duplicate = "duplicate"
)

// Statuses lists them in the order the counts show.
var Statuses = []string{Reading, Created, Added, Unread, Failed, Duplicate}

// Counts tallies items by status.
func Counts(statuses []string) map[string]int {
	out := make(map[string]int, len(Statuses))
	for _, s := range Statuses {
		out[s] = 0
	}
	for _, s := range statuses {
		out[s]++
	}
	return out
}
