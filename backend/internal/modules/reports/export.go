package reports

// The reports as a file: in Excel a sheet each, in CSV one section each.
// Not audited: reports are counts and totals, not records of people.

import (
	"bytes"
	"context"
	"mime"
	"strconv"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

var stageLabel = map[string]string{
	"applied": "Applied", "screening": "Screening", "submitted": "Submitted", "interview": "Interview",
	"offer": "Offer", "placed": "Placed", "rejected": "Rejected", "withdrawn": "Withdrawn",
}

// Tables are the report's sheets.
func Tables(r Report) []sheet.Table {
	funnel := sheet.Table{Name: "Pipeline", Columns: []sheet.Column{{Heading: "Stage"}, {Heading: "Reached", Kind: sheet.Number}, {Heading: "% of the stage before", Kind: sheet.Number}}}
	for _, f := range r.Funnel {
		pct := ""
		if f.PercentOfPrevious != nil {
			pct = strconv.Itoa(*f.PercentOfPrevious)
		}
		name := f.Label
		if name == "" {
			name = stageLabel[f.Stage]
		}
		funnel.Rows = append(funnel.Rows, []string{name, strconv.Itoa(f.Reached), pct})
	}
	sources := sheet.Table{Name: "Sources", Columns: []sheet.Column{{Heading: "Source"}, {Heading: "Applications", Kind: sheet.Number},
		{Heading: "Interviewed", Kind: sheet.Number}, {Heading: "Placed", Kind: sheet.Number}}}
	for _, s := range r.Sources {
		sources.Rows = append(sources.Rows, []string{sourceText(s.Source, r.SourceNames), strconv.Itoa(s.Applications), strconv.Itoa(s.Interviewed), strconv.Itoa(s.Placed)})
	}
	cols := []sheet.Column{{Heading: "Recruiter"}, {Heading: "Placements", Kind: sheet.Number}, {Heading: "Fell through", Kind: sheet.Number}}
	if !r.FeesHidden {
		cols = append(cols, sheet.Column{Heading: "Fees"})
	}
	recruiters := sheet.Table{Name: "Recruiters", Columns: cols}
	for _, rr := range r.Recruiters {
		name := "No recruiter credited"
		if rr.RecruiterID != nil {
			name = r.RecruiterNames[*rr.RecruiterID]
		}
		row := []string{name, strconv.Itoa(rr.Placements), strconv.Itoa(rr.FellThrough)}
		if !r.FeesHidden {
			var fees []string
			for _, f := range rr.Fees {
				fees = append(fees, f.Currency+" "+f.Amount)
			}
			row = append(row, strings.Join(fees, ", "))
		}
		recruiters.Rows = append(recruiters.Rows, row)
	}
	fill := sheet.Table{Name: "Time to fill", Columns: []sheet.Column{{Heading: "Job"}, {Heading: "Days to fill", Kind: sheet.Number}, {Heading: "Placements", Kind: sheet.Number}}}
	for _, j := range r.TimeToFill.Jobs {
		fill.Rows = append(fill.Rows, []string{r.JobTitles[j.JobID], strconv.Itoa(j.DaysToFill), strconv.Itoa(j.Placements)})
	}
	return []sheet.Table{funnel, sources, recruiters, fill}
}

// sourceText is a source as Settings names it, else its key in words.
func sourceText(s string, names map[string]string) string {
	if s == "" || s == "unknown" {
		return "Not recorded"
	}
	if name := names[s]; name != "" {
		return name
	}
	words := strings.ReplaceAll(s, "_", " ")
	return strings.ToUpper(words[:1]) + words[1:]
}

// ExportReport handles GET /v1/reports/summary/export.
func (h *HTTP) ExportReport(ctx context.Context, req api.ExportReportRequestObject) (api.ExportReportResponseObject, error) {
	p := authz.MustFromContext(ctx)
	r, err := h.svc.Report(ctx, p, day(req.Params.From), day(req.Params.To))
	if err != nil {
		return nil, err
	}
	activity, forecast, err := h.svc.Extras(ctx, p, r.From, r.To)
	if err != nil {
		return nil, err
	}
	tables := append(Tables(r), ActivityTable(activity))
	if forecast != nil {
		tables = append(tables, ForecastTables(*forecast)...)
	}
	name := "Reports " + r.From.Format("2 Jan 2006") + " to " + r.To.Format("2 Jan 2006")
	file, err := sheet.Write(string(req.Params.Format), name, tables...)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	return api.ExportReport200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(file.Data), ContentLength: int64(len(file.Data)),
		Headers: api.ExportReport200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
