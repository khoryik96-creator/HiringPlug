package bin

import (
	"encoding/json"
	"fmt"
	"time"
)

// Keep is how long a record waits in the Bin before it is deleted for good.
const Keep = 30 * 24 * time.Hour

// WentWith is what went to the Bin with a company.
type WentWith struct {
	Contacts []string `json:"contacts,omitempty"`
	Jobs     []string `json:"jobs,omitempty"`
}

func (w WentWith) encode() []byte {
	b, _ := json.Marshal(w) // two string slices: cannot fail
	return b
}

func decodeWentWith(b []byte) WentWith {
	var w WentWith
	_ = json.Unmarshal(b, &w) // written by encode
	return w
}

// placedReason says why a record with placements cannot be binned.
func placedReason(n int) string {
	if n == 1 {
		return "It has a placement: placements and their invoices are kept, so it cannot be deleted."
	}
	return fmt.Sprintf("It has %d placements: placements and their invoices are kept, so it cannot be deleted.", n)
}

// unbinnable returns the applications of these that may be shown again: those
// whose candidate and job are both out of the Bin.
func unbinnable(apps []Application, binnedCandidates, binnedJobs []string) []string {
	hidden := map[string]bool{}
	for _, id := range append(append([]string{}, binnedCandidates...), binnedJobs...) {
		hidden[id] = true
	}
	var out []string
	for _, a := range apps {
		if !hidden[a.CandidateID] && !hidden[a.JobID] {
			out = append(out, a.ID)
		}
	}
	return out
}

func appIDs(apps []Application) []string {
	out := make([]string, len(apps))
	for i, a := range apps {
		out[i] = a.ID
	}
	return out
}
