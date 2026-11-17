package candidates

// A work authorization suggested from a candidate's CV: what it says of their
// nationality and of a pass they hold. Shown striped on their card until
// someone confirms or sets one; never set on its own.

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/country"
)

// WorkAuthSuggestion is a suggested work authorization and why.
type WorkAuthSuggestion struct {
	Status string  `json:"status"`
	Basis  *string `json:"basis,omitempty"`
	Reason string  `json:"reason"`
}

var (
	cvRPT    = regexp.MustCompile(`(?i)\bRP[- ]?T\b|residence pass[- ]talent`)
	cvSpouse = regexp.MustCompile(`(?i)\bspouse(?:'s)? (?:visa|pass)\b|\bdependant(?:'s)? pass\b|\bdependent(?:'s)? pass\b|long[- ]term social visit pass|\bLTSVP\b`)
	cvPR     = regexp.MustCompile(`(?i)\bpermanent resident\b|\bPR (?:holder|status)\b`)
)

// SuggestWorkAuth reads a suggestion from a CV: its nationality (an ISO
// code), and a pass it mentions. home is the agency's country. Nil when the
// CV does not say enough.
func SuggestWorkAuth(nationality, cvText, home string) *WorkAuthSuggestion {
	expat := func(basis, reason string) *WorkAuthSuggestion {
		return &WorkAuthSuggestion{Status: "expat_eligible", Basis: &basis, Reason: reason}
	}
	switch {
	case nationality != "" && nationality == home:
		return &WorkAuthSuggestion{Status: "local", Reason: "Nationality on the CV: " + country.Name(nationality)}
	case cvRPT.MatchString(cvText):
		return expat("rp_t", "The CV mentions an RP-T (Residence Pass-Talent)")
	case cvSpouse.MatchString(cvText):
		return expat("spouse_visa", "The CV mentions a spouse or dependant pass")
	case cvPR.MatchString(cvText):
		return expat("permanent_resident", "The CV mentions permanent residency")
	case nationality != "":
		return &WorkAuthSuggestion{Status: "requires_visa", Reason: "Nationality on the CV: " + country.Name(nationality) + ", and no pass mentioned"}
	}
	return nil
}

// SuggestWorkAuthFromCV keeps a suggestion read from a candidate's CV, while
// they have no work authorization set, in the caller's transaction.
func (m *Module) SuggestWorkAuthFromCV(ctx context.Context, tx pgx.Tx, candidateID, nationality, cvText string) error {
	s := SuggestWorkAuth(nationality, cvText, m.service.phoneRegion)
	if s == nil {
		return nil
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := store.New(tx).SetWorkAuthSuggestion(ctx, store.SetWorkAuthSuggestionParams{ID: candidateID, Suggestion: raw}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "candidate.work_authorization_suggested", EntityType: "candidate", EntityID: candidateID,
		Changes: map[string]any{"status": s.Status, "basis": s.Basis, "reason": s.Reason}})
}

func suggestionOf(raw []byte) *WorkAuthSuggestion {
	if len(raw) == 0 {
		return nil
	}
	var s WorkAuthSuggestion
	if json.Unmarshal(raw, &s) != nil || s.Status == "" {
		return nil
	}
	return &s
}
