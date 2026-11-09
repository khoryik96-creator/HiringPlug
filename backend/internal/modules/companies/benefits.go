package companies

// A client's benefits and its steps before an offer, filled in from the
// agency's template (the company_benefits and pre_offer_process lists in
// Settings, as in the Hyppies Benefits & Pre-Offer Process template).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

// Benefit is one item of a client's benefits.
type Benefit struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Value   *string `json:"value"`
	Remarks *string `json:"remarks"`
}

// PreOfferStep is one step a client may take before an offer.
type PreOfferStep struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Applies *bool   `json:"applies"` // nil: not known yet
	Remarks *string `json:"remarks"`
}

// Benefits is a client's benefits and pre-offer process, as last saved.
type Benefits struct {
	Benefits  []Benefit
	PreOffer  []PreOfferStep
	Version   int // 0 before the first save
	UpdatedAt *time.Time
}

// tidyText trims s, keeping its lines; empty is nil.
func tidyText(s *string, limit int, field string, errs *apperr.FieldErrors) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(strings.ReplaceAll(*s, "\r\n", "\n"))
	if t == "" {
		return nil
	}
	if len([]rune(t)) > limit {
		errs.Add(field, fmt.Sprintf("must be at most %d characters", limit))
	}
	return &t
}

// NormalizeBenefits checks what a person saves: items named, each once.
func NormalizeBenefits(b Benefits) (Benefits, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	item := func(field, key, label string, seen map[string]bool) (string, string) {
		key, label = strings.TrimSpace(key), strings.Join(strings.Fields(label), " ")
		switch {
		case key == "":
			errs.Add(field+".key", "is required")
		case seen[key]:
			errs.Add(field+".key", "is already in the list")
		}
		if label == "" {
			errs.Add(field+".label", "is required")
		}
		seen[key] = true
		return key, label
	}
	seen := map[string]bool{}
	for i := range b.Benefits {
		f := fmt.Sprintf("benefits[%d]", i)
		x := &b.Benefits[i]
		x.Key, x.Label = item(f, x.Key, x.Label, seen)
		x.Value = tidyText(x.Value, 1000, f+".value", &errs)
		x.Remarks = tidyText(x.Remarks, 2000, f+".remarks", &errs)
	}
	seen = map[string]bool{}
	for i := range b.PreOffer {
		f := fmt.Sprintf("preOffer[%d]", i)
		x := &b.PreOffer[i]
		x.Key, x.Label = item(f, x.Key, x.Label, seen)
		x.Remarks = tidyText(x.Remarks, 2000, f+".remarks", &errs)
	}
	return b, errs
}

// GetBenefits returns a client's benefits (empty before the first save).
func (s *Service) GetBenefits(ctx context.Context, p authz.Principal, companyID string) (Benefits, error) {
	var out Benefits
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.benefitsIn(ctx, tx, companyID)
		return err
	})
	return out, err
}

func (s *Service) benefitsIn(ctx context.Context, tx pgx.Tx, companyID string) (Benefits, error) {
	q := store.New(tx)
	exists, err := q.CompanyExists(ctx, companyID)
	if err != nil {
		return Benefits{}, err
	}
	if !exists {
		return Benefits{}, apperr.NotFound("company")
	}
	row, err := q.GetCompanyBenefits(ctx, companyID)
	if database.IsNoRows(err) {
		return Benefits{Benefits: []Benefit{}, PreOffer: []PreOfferStep{}}, nil
	}
	if err != nil {
		return Benefits{}, err
	}
	out := Benefits{Version: int(row.Version), UpdatedAt: &row.UpdatedAt}
	if err := json.Unmarshal(row.Benefits, &out.Benefits); err != nil {
		return Benefits{}, err
	}
	return out, json.Unmarshal(row.PreOffer, &out.PreOffer)
}

// PutBenefits saves a client's benefits and pre-offer process (the whole
// lists). expectedVersion is the version the person loaded (0: none yet).
func (s *Service) PutBenefits(ctx context.Context, p authz.Principal, companyID string, expectedVersion int, in Benefits) (Benefits, error) {
	in, errs := NormalizeBenefits(in)
	if err := errs.Err(); err != nil {
		return Benefits{}, err
	}
	benefits, err := json.Marshal(in.Benefits)
	if err != nil {
		return Benefits{}, err
	}
	preOffer, err := json.Marshal(in.PreOffer)
	if err != nil {
		return Benefits{}, err
	}
	var out Benefits
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		before, err := s.benefitsIn(ctx, tx, companyID)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return apperr.StaleVersion("benefits").WithDetail("currentVersion", before.Version)
		}
		q := store.New(tx)
		var version int32
		var at time.Time
		if expectedVersion == 0 {
			row, err := q.InsertCompanyBenefits(ctx, store.InsertCompanyBenefitsParams{CompanyID: companyID, Benefits: benefits, PreOffer: preOffer})
			if database.IsNoRows(err) {
				return apperr.StaleVersion("benefits") // saved by someone else meanwhile
			}
			if err != nil {
				return err
			}
			version, at = row.Version, row.UpdatedAt
		} else {
			row, err := q.UpdateCompanyBenefits(ctx, store.UpdateCompanyBenefitsParams{
				CompanyID: companyID, Benefits: benefits, PreOffer: preOffer, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small
			})
			if database.IsNoRows(err) {
				return apperr.StaleVersion("benefits")
			}
			if err != nil {
				return err
			}
			version, at = row.Version, row.UpdatedAt
		}
		out = Benefits{Benefits: in.Benefits, PreOffer: in.PreOffer, Version: int(version), UpdatedAt: &at}
		// Which items changed, not what they say (bonus and pay terms stay out of the audit log).
		return audit.Record(ctx, tx, audit.Entry{Action: "company.benefits_saved", EntityType: "company", EntityID: companyID,
			Changes: map[string]any{"changed": changedItems(before, in)}})
	})
	return out, err
}

// changedItems are the labels of the items whose answers changed.
func changedItems(before, after Benefits) []string {
	was := map[string]string{}
	for _, b := range before.Benefits {
		was["b:"+b.Key] = deref(b.Value) + "\x00" + deref(b.Remarks)
	}
	for _, s := range before.PreOffer {
		was["p:"+s.Key] = fmt.Sprint(s.Applies != nil && *s.Applies, s.Applies == nil) + "\x00" + deref(s.Remarks)
	}
	out := []string{}
	for _, b := range after.Benefits {
		if v, ok := was["b:"+b.Key]; (ok || b.Value != nil || b.Remarks != nil) && v != deref(b.Value)+"\x00"+deref(b.Remarks) {
			out = append(out, b.Label)
		}
	}
	for _, s := range after.PreOffer {
		now := fmt.Sprint(s.Applies != nil && *s.Applies, s.Applies == nil) + "\x00" + deref(s.Remarks)
		if v, ok := was["p:"+s.Key]; (ok || s.Applies != nil || s.Remarks != nil) && v != now {
			out = append(out, s.Label)
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// BenefitsExport is a client's benefits as a file to download.
type BenefitsExport struct {
	FileName string
	Data     []byte
}

// ExportBenefits writes a client's benefits and pre-offer process as a
// Word file or a PDF, in two tables as in the template.
func (s *Service) ExportBenefits(ctx context.Context, p authz.Principal, companyID, format string) (BenefitsExport, error) {
	var b Benefits
	var name string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		names, err := store.New(tx).CompanyNames(ctx, []string{companyID})
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return apperr.NotFound("company")
		}
		name = names[0].Name
		b, err = s.benefitsIn(ctx, tx, companyID)
		return err
	})
	if err != nil {
		return BenefitsExport{}, err
	}
	doc := BenefitsDoc(name, b)
	out := BenefitsExport{FileName: fileSafe(name) + " - Benefits." + format}
	if format == "pdf" {
		out.Data, err = doc.PDF()
	} else {
		out.Data, err = doc.Docx()
	}
	return out, err
}

// BenefitsDoc lays a client's benefits out as the template does.
func BenefitsDoc(company string, b Benefits) simpledoc.Doc {
	doc := simpledoc.Doc{{Kind: simpledoc.Title, Text: company + ": benefits"}}
	if b.UpdatedAt != nil {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Muted, Text: "As at " + b.UpdatedAt.Format("2 January 2006")})
	}
	doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Benefits"},
		simpledoc.Block{Kind: simpledoc.HeaderRow, Cells: []string{"Item", "Value", "Remarks or clauses"}})
	for _, x := range b.Benefits {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Row, Cells: []string{x.Label, deref(x.Value), deref(x.Remarks)}})
	}
	doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: "Pre-offer process"},
		simpledoc.Block{Kind: simpledoc.HeaderRow, Cells: []string{"Item", "Applies", "Remarks"}})
	for _, x := range b.PreOffer {
		applies := ""
		if x.Applies != nil {
			applies = map[bool]string{true: "Yes", false: "No"}[*x.Applies]
		}
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Row, Cells: []string{x.Label, applies, deref(x.Remarks)}})
	}
	return doc
}

// fileSafe is a name fit for a file name.
func fileSafe(name string) string {
	out := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 0x20 {
			return ' '
		}
		return r
	}, name)
	return strings.Join(strings.Fields(out), " ")
}
