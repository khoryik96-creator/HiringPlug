package scheduling

// The owner's view of everyone's booking set-up: their hours, whether their
// calendar is connected and read (its host only: the address is private to
// them) and their booking links.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Member is an active user.
type Member struct{ ID, Name string }

// TeamSchedule is one person's booking set-up.
type TeamSchedule struct {
	UserID, Name      string
	Schedule          Schedule
	OpenLinks, Coming int
	NextAt            *time.Time
}

// TeamSchedules returns every active user's booking set-up, by name.
func (s *Service) TeamSchedules(ctx context.Context, p authz.Principal) ([]TeamSchedule, error) {
	var out []TeamSchedule
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		members, err := s.deps.Users.Members(ctx, tx)
		if err != nil {
			return err
		}
		counts, err := q.HostLinkCounts(ctx)
		if err != nil {
			return err
		}
		byHost := make(map[string]store.HostLinkCountsRow, len(counts))
		for _, c := range counts {
			byHost[c.HostUserID] = c
		}
		saved, err := q.ListSchedules(ctx)
		if err != nil {
			return err
		}
		schedules := make(map[string]store.Schedule, len(saved))
		for _, r := range saved {
			schedules[r.UserID] = r
		}
		busyRows, err := q.BusyCounts(ctx)
		if err != nil {
			return err
		}
		busy := make(map[string]int, len(busyRows))
		for _, b := range busyRows {
			busy[b.UserID] = int(b.Busy)
		}
		out = make([]TeamSchedule, 0, len(members))
		for _, m := range members {
			sched := Schedule{Settings: DefaultSettings(s.settings.DefaultTimeZone.String())}
			if row, ok := schedules[m.ID]; ok {
				if sched, err = scheduleFrom(row, busy[m.ID]); err != nil {
					return err
				}
			}
			c := byHost[m.ID]
			t := TeamSchedule{UserID: m.ID, Name: m.Name, Schedule: sched, OpenLinks: int(c.OpenLinks), Coming: int(c.Upcoming)}
			if c.Upcoming > 0 {
				next := c.NextAt
				t.NextAt = &next
			}
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

// ListTeamSchedules handles GET /v1/scheduling/team.
func (h *HTTP) ListTeamSchedules(ctx context.Context, _ api.ListTeamSchedulesRequestObject) (api.ListTeamSchedulesResponseObject, error) {
	rows, err := h.svc.TeamSchedules(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListTeamSchedules200JSONResponse{Items: make([]api.TeamSchedule, len(rows))}
	for i, r := range rows {
		mine := scheduleToAPI(r.Schedule)
		out.Items[i] = api.TeamSchedule{UserId: r.UserID, Name: r.Name, Set: mine.Set, TimeZone: mine.TimeZone, Hours: mine.Hours,
			Calendar: mine.Calendar, OpenLinks: r.OpenLinks, Upcoming: r.Coming}
		out.Items[i].NextAt.SetNull()
		if r.NextAt != nil {
			out.Items[i].NextAt.Set(*r.NextAt)
		}
	}
	return out, nil
}
