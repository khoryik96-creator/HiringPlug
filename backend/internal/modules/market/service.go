package market

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the module's use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	now  func() time.Time
}

// Map is a job's market map.
type Map struct {
	JobID, JobTitle, SearchedFor string
	Pool                         Pool
	SalaryRestricted             bool
	Insights                     *Answer
}

// Map gathers a job's market map as p may see it.
func (s *Service) Map(ctx context.Context, p authz.Principal, jobID string) (Map, error) {
	var out Map
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.mapIn(ctx, tx, p, jobID)
		return err
	})
	return out, err
}

func (s *Service) mapIn(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (Map, error) {
	title, err := s.deps.Jobs.Title(ctx, tx, p, jobID)
	if err != nil {
		return Map{}, err
	}
	out := Map{JobID: jobID, JobTitle: title, SearchedFor: TitleWords(title), SalaryRestricted: !p.Can(api.PermissionCandidatesCompensationRead)}
	members, ids, err := s.deps.Candidates.Members(ctx, tx, p, out.SearchedFor)
	if err != nil {
		return Map{}, err
	}
	var pays []Pay
	if !out.SalaryRestricted {
		if pays, err = s.deps.Candidates.Pay(ctx, tx, p, ids); err != nil {
			return Map{}, err
		}
	}
	out.Pool = Build(members, pays)
	out.Insights, err = s.deps.Insights.Latest(ctx, tx, jobID)
	return out, err
}

// Export is a downloaded market map.
type Export struct {
	FileName, ContentType string
	Data                  []byte
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Export lays the market map out as a Word ("docx") or PDF ("pdf") file.
func (s *Service) Export(ctx context.Context, p authz.Principal, jobID, format string) (Export, error) {
	m, err := s.Map(ctx, p, jobID)
	if err != nil {
		return Export{}, err
	}
	var insight *Insight
	if a := m.Insights; a != nil && a.Status == "done" && a.Text != nil {
		insight = &Insight{Text: *a.Text, AskedAt: a.AskedAt}
	}
	doc := Doc(m.JobTitle, m.SearchedFor, m.Pool, m.SalaryRestricted, insight, s.now())
	name := strings.TrimSpace(unsafeName.ReplaceAllString(m.JobTitle, " "))
	if name == "" {
		name = "Job"
	}
	out := Export{FileName: fmt.Sprintf("Market map - %s.%s", name, format)}
	if format == "pdf" {
		out.ContentType = "application/pdf"
		out.Data, err = doc.PDF()
	} else {
		out.ContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		out.Data, err = doc.Docx()
	}
	return out, err
}
