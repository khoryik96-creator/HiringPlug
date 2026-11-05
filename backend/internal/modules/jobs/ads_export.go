package jobs

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/country"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

// Export is a downloaded ad.
type Export struct {
	FileName string
	Data     []byte
}

// AdDoc lays an ad out as a document: its title, a line of facts, its text.
// facts are the words for the work type and arrangement.
func AdDoc(title string, text *string, d AdDetails, workType, arrangement string) simpledoc.Doc {
	var facts []string
	place := ""
	if d.Location != nil {
		place = *d.Location
	}
	if d.Country != nil {
		place = strings.TrimPrefix(place+", "+country.Name(*d.Country), ", ")
	}
	for _, f := range []string{place, workType, arrangement, deref(d.SalaryText)} {
		if f != "" {
			facts = append(facts, f)
		}
	}
	if d.ClosesOn != nil {
		facts = append(facts, "Apply by "+d.ClosesOn.Format("2 Jan 2006"))
	}
	doc := simpledoc.Doc{{Kind: simpledoc.Title, Text: title}}
	if len(facts) > 0 {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Muted, Text: strings.Join(facts, " · ")})
	}
	if text != nil {
		doc = append(doc, simpledoc.FromText(*text)...)
	}
	return doc
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// ExportFileName is a tidy file name for an ad: "Finance Manager (blind).pdf".
func ExportFileName(title, version, format string) string {
	name := strings.Trim(unsafeName.ReplaceAllString(title, " "), " ")
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	if name == "" {
		name = "Job ad"
	}
	if version == "blind" {
		name += " (blind)"
	}
	return fmt.Sprintf("%s.%s", name, format)
}

// ExportAd returns an ad, or its blind version, as a Word file or a PDF.
func (s *Service) ExportAd(ctx context.Context, p authz.Principal, id, version, format string) (Export, error) {
	var out Export
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getAdRow(ctx, store.New(tx), id)
		if err != nil {
			return err
		}
		ad := adFromRow(row)
		if err := s.withBlind(ctx, tx, &ad); err != nil {
			return err
		}
		title, text, facts := ad.Details.Title, ad.Details.Description, ad.Details
		if version == "blind" {
			title, text = ad.Blind.Title, ad.Blind.Description
			facts.Location, facts.SalaryText = ad.Blind.Location, ad.Blind.SalaryText
		}
		labels := map[string]map[string]string{}
		if s.rules != nil {
			if labels, err = s.rules.Labels(ctx, tx); err != nil {
				return err
			}
		}
		label := func(list string, key *string) string {
			if l, ok := labels[list][deref(key)]; ok {
				return l
			}
			return deref(key)
		}
		workType, arrangement := label("work_type", ad.Details.WorkType), label("work_arrangement", ad.Details.WorkArrangement)
		doc := AdDoc(title, text, facts, workType, arrangement)
		if format == "pdf" {
			out.Data, err = doc.PDF()
		} else {
			out.Data, err = doc.Docx()
		}
		if err != nil {
			return err
		}
		out.FileName = ExportFileName(title, version, format)
		return audit.Record(ctx, tx, audit.Entry{
			Action: "job_ad.exported", EntityType: "job_ad", EntityID: id, Changes: map[string]any{"version": version, "format": format},
		})
	})
	return out, err
}
