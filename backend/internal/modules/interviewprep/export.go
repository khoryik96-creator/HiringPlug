package interviewprep

// Prep material as a Word or PDF file, to send to a candidate: the same
// short cards as on screen, each question with its labelled parts.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

// Export is a file made from prep material.
type Export struct {
	FileName    string
	ContentType string
	Data        []byte
}

var unsafeName = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// ExportPrep makes prep material a Word ("docx") or PDF ("pdf") file.
func (s *Service) ExportPrep(ctx context.Context, p authz.Principal, id, format string) (Export, error) {
	var prep Prep
	var job Job
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetInterviewPrep(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("interview prep")
		}
		if err != nil {
			return err
		}
		if job, err = s.deps.Jobs.ForInterviewPrep(ctx, tx, p, row.JobID); err != nil {
			return apperr.NotFound("interview prep")
		}
		prep = prepFrom(row)
		return nil
	})
	if err != nil {
		return Export{}, err
	}
	if prep.Content == nil {
		return Export{}, apperr.New(api.ErrorCodeINTERVIEWPREPNOTWRITTEN, "There is nothing to export yet: the AI has not written this prep.")
	}
	doc := PrepDoc(job.Title, job.CompanyName, prep, s.now().In(s.zone))
	name := strings.TrimSpace(unsafeName.ReplaceAllString(job.Title, " "))
	if name == "" {
		name = "Job"
	}
	out := Export{FileName: fmt.Sprintf("Interview prep - %s.%s", name, format)}
	if format == "pdf" {
		out.ContentType = "application/pdf"
		out.Data, err = doc.PDF()
	} else {
		out.ContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		out.Data, err = doc.Docx()
	}
	return out, err
}

// PrepDoc is prep material laid out to read: a short summary, then a card
// per question (why they ask, how to approach it, example lines set apart,
// trade-offs and what to avoid), and questions to ask them.
func PrepDoc(title, company string, p Prep, at time.Time) simpledoc.Doc {
	c := p.Content
	meta := []string{}
	if company != "" {
		meta = append(meta, company)
	}
	meta = append(meta, at.Format("2 Jan 2006"))
	if p.Focus != nil {
		meta = append(meta, "Focus: "+*p.Focus)
	}
	doc := simpledoc.Doc{
		{Kind: simpledoc.Title, Text: "Interview prep: " + title},
		{Kind: simpledoc.Muted, Text: strings.Join(meta, " · ")},
	}
	if c == nil {
		return doc
	}
	if c.Summary != "" {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Paragraph, Text: c.Summary})
	}
	doc = append(doc, listDoc("Brush up on", c.FocusAreas)...)
	doc = append(doc, itemsDoc("Soft skills: situational, what-if and behavioural", c.SoftSkills)...)
	doc = append(doc, itemsDoc("Technical", c.Technical)...)
	if len(c.QuestionsToAsk) > 0 {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Section, Text: "Questions to ask them"})
		for _, q := range c.QuestionsToAsk {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: q})
		}
	}
	return append(doc, simpledoc.Block{Kind: simpledoc.Muted,
		Text: "Guidance on how to approach each question, not answers to learn by heart: put it in your own words and your own examples."})
}

func itemsDoc(section string, in []api.InterviewPrepItem) simpledoc.Doc {
	if len(in) == 0 {
		return nil
	}
	doc := simpledoc.Doc{{Kind: simpledoc.Section, Text: section}}
	for i, it := range in {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Heading, Text: fmt.Sprintf("%d. %s", i+1, it.Question)})
		about := []string{it.Kind}
		if it.FromInterviews {
			about = append(about, "Asked at this job's interviews")
		}
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Muted, Text: strings.Join(about, " · ")})
		if it.WhyAsked != "" {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Label, Text: "Why they ask"}, simpledoc.Block{Kind: simpledoc.Paragraph, Text: it.WhyAsked})
		}
		doc = append(doc, listDoc("How to approach", it.Approach)...)
		if len(it.Phrasing) > 0 {
			doc = append(doc, simpledoc.Block{Kind: simpledoc.Label, Text: "Say it like"})
			for _, line := range it.Phrasing {
				doc = append(doc, simpledoc.Block{Kind: simpledoc.Quote, Text: "“" + strings.Trim(line, `"“”`) + "”"})
			}
		}
		doc = append(doc, listDoc("Trade-offs", it.TradeOffs)...)
		doc = append(doc, listDoc("Avoid", it.Pitfalls)...)
	}
	return doc
}

func listDoc(label string, lines []string) simpledoc.Doc {
	if len(lines) == 0 {
		return nil
	}
	doc := simpledoc.Doc{{Kind: simpledoc.Label, Text: label}}
	for _, l := range lines {
		doc = append(doc, simpledoc.Block{Kind: simpledoc.Bullet, Text: l})
	}
	return doc
}
