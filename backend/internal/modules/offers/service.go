package offers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the offer use cases.
type Service struct {
	pool         *pgxpool.Pool
	applications Applications
	users        Users
}

// Salary is the salary offered.
type Salary struct{ Amount, Currency, Period string }

// View is an offer as a user may see it.
type View struct {
	ID, ApplicationID string
	Version           int
	App               Application
	Offer             Offer
	Salary            *Salary // nil: none set, or hidden (SalaryHidden)
	SalaryHidden      bool
	Names             map[string]string // user id -> name
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

const oneOpenOffer = "offer_open_idx"

func notAllowed(message string) error { return apperr.New(api.ErrorCodeOFFERNOTALLOWED, message) }

func maySetSalary(p authz.Principal) error {
	if !p.Can(api.PermissionOffersSalaryUpdate) {
		return apperr.New(api.ErrorCodeFORBIDDEN, "You cannot set the salary of an offer.")
	}
	return nil
}

// Create drafts an offer for an application and moves it to the offer stage.
func (s *Service) Create(ctx context.Context, p authz.Principal, appID string, terms TermsPatch, salary *Salary) (View, error) {
	if salary != nil {
		if err := maySetSalary(p); err != nil {
			return View{}, err
		}
	}
	o, _ := Edit(Offer{Status: Draft}, terms)
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications.Briefs(ctx, tx, []string{appID})
		if err != nil {
			return err
		}
		app, ok := apps[appID]
		if !ok {
			return apperr.NotFound("application")
		}
		switch app.Stage {
		case "rejected", "withdrawn":
			return notAllowed("Reopen the application (move it to an open stage) before making an offer.")
		case "placed":
			return notAllowed("The candidate is already placed in this job.")
		}
		q := store.New(tx)
		row, err := q.InsertOffer(ctx, store.InsertOfferParams{ApplicationID: appID, StartDate: o.StartDate, ExpiresOn: o.ExpiresOn, OfferTerms: o.Terms})
		if database.IsUniqueViolation(err, oneOpenOffer) {
			return notAllowed("This application already has an open offer: change that one, or withdraw it first.")
		}
		if err != nil {
			return err
		}
		if err := s.applications.ReachOffer(ctx, tx, appID); err != nil {
			return err
		}
		if salary != nil {
			if err := q.PutOfferSalary(ctx, store.PutOfferSalaryParams{OfferID: row.ID, Amount: salary.Amount, Currency: salary.Currency, Period: salary.Period}); err != nil {
				return err
			}
		}
		changes := map[string]any{"applicationId": appID}
		if salary != nil {
			changes["salary"] = audit.Redacted
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "offer.drafted", EntityType: "offer", EntityID: row.ID, Changes: changes}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// change loads an offer, checks its version, applies fn and saves it with an
// audit entry, in one transaction.
func (s *Service) change(ctx context.Context, p authz.Principal, id string, version int, action string,
	fn func(tx pgx.Tx, q *store.Queries, o Offer) (Offer, map[string]any, error),
) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetOffer(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("offer")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != version {
			return apperr.StaleVersion("offer").WithDetail("currentVersion", row.Version)
		}
		before := offerOf(row)
		after, changes, err := fn(tx, q, before)
		if err != nil {
			return err
		}
		updated, err := q.UpdateOffer(ctx, store.UpdateOfferParams{
			ID: id, ExpectedVersion: int32(version), Status: after.Status, //nolint:gosec // versions are small
			StartDate: after.StartDate, ExpiresOn: after.ExpiresOn, Terms: after.Terms,
			RequestedBy: after.RequestedBy, DecidedBy: after.DecidedBy, DecidedAt: after.DecidedAt, DecisionNote: after.DecisionNote,
			SentAt: after.SentAt, AnsweredAt: after.AnsweredAt,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("offer")
		}
		if err != nil {
			return err
		}
		if before.Status != after.Status {
			changes["status"] = map[string]string{"from": before.Status, "to": after.Status}
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "offer", EntityID: id, Changes: changes}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, updated)
		return err
	})
	return out, err
}

// Update changes an offer's terms and (when set) its salary.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, version int, terms TermsPatch, salary apiconv.Change[*Salary]) (View, error) {
	if salary.Set {
		if err := maySetSalary(p); err != nil {
			return View{}, err
		}
	}
	return s.change(ctx, p, id, version, "offer.changed", func(_ pgx.Tx, q *store.Queries, o Offer) (Offer, map[string]any, error) {
		after, problem := Edit(o, terms)
		if problem != "" {
			return o, nil, notAllowed(problem)
		}
		changes := audit.Diff(termFields(o), termFields(after))
		if salary.Set {
			changes["salary"] = audit.Redacted // never the amount
			var err error
			if v := salary.Value; v != nil {
				err = q.PutOfferSalary(ctx, store.PutOfferSalaryParams{OfferID: id, Amount: v.Amount, Currency: v.Currency, Period: v.Period})
			} else {
				err = q.DeleteOfferSalary(ctx, id)
			}
			if err != nil {
				return o, nil, err
			}
			if after.Status == PendingApproval || after.Status == Approved {
				after.Status, after.DecidedBy, after.DecidedAt = Draft, nil, nil
			}
		}
		return after, changes, nil
	})
}

// Step moves an offer on (ask for approval, send, record the answer, withdraw).
func (s *Service) Step(ctx context.Context, p authz.Principal, id string, version int, step string) (View, error) {
	return s.change(ctx, p, id, version, "offer."+stepAction[step], func(_ pgx.Tx, q *store.Queries, o Offer) (Offer, map[string]any, error) {
		salaries, err := q.OfferSalaries(ctx, []string{id})
		if err != nil {
			return o, nil, err
		}
		after, problem := Step(o, step, p.UserID, p.Can(api.PermissionOffersApprove), len(salaries) > 0, time.Now())
		if problem != "" {
			return o, nil, notAllowed(problem)
		}
		return after, map[string]any{}, nil
	})
}

var stepAction = map[string]string{
	StepRequestApproval: "approval_requested", StepSend: "sent", StepAccept: "accepted", StepDecline: "declined", StepWithdraw: "withdrawn",
}

// Decide approves an offer waiting for approval, or sends it back to draft.
func (s *Service) Decide(ctx context.Context, p authz.Principal, id string, version int, approve bool, note *string) (View, error) {
	action := "offer.sent_back"
	if approve {
		action = "offer.approved"
	}
	return s.change(ctx, p, id, version, action, func(_ pgx.Tx, _ *store.Queries, o Offer) (Offer, map[string]any, error) {
		after, problem := Decide(o, approve, note, p.UserID, time.Now())
		if problem != "" {
			return o, nil, notAllowed(problem)
		}
		return after, map[string]any{"note": after.DecisionNote}, nil
	})
}

// List returns offers, newest first.
func (s *Service) List(ctx context.Context, p authz.Principal, appID, status *string) ([]View, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListOffers(ctx, store.ListOffersParams{ApplicationID: appID, Status: status})
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, p, rows)
		return err
	})
	return out, err
}

func offerOf(r store.Offer) Offer {
	return Offer{
		Status: r.Status, StartDate: r.StartDate, ExpiresOn: r.ExpiresOn, Terms: r.Terms,
		RequestedBy: r.RequestedBy, DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
		SentAt: r.SentAt, AnsweredAt: r.AnsweredAt,
	}
}

func termFields(o Offer) map[string]any {
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format(time.DateOnly)
	}
	return map[string]any{"startDate": day(o.StartDate), "expiresOn": day(o.ExpiresOn), "terms": o.Terms}
}

func (s *Service) one(ctx context.Context, tx pgx.Tx, p authz.Principal, row store.Offer) (View, error) {
	views, err := s.views(ctx, tx, p, []store.Offer{row})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// views adds the application, the names of who asked and decided, and (when
// the user may see it) the salary.
func (s *Service) views(ctx context.Context, tx pgx.Tx, p authz.Principal, rows []store.Offer) ([]View, error) {
	appIDs, ids, userIDs := make([]string, len(rows)), make([]string, len(rows)), []string{}
	for i, r := range rows {
		appIDs[i], ids[i] = r.ApplicationID, r.ID
		for _, u := range []*string{r.RequestedBy, r.DecidedBy} {
			if u != nil {
				userIDs = append(userIDs, *u)
			}
		}
	}
	apps, err := s.applications.Briefs(ctx, tx, appIDs)
	if err != nil {
		return nil, err
	}
	names, err := s.users.UserNames(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	seeSalary := p.Can(api.PermissionOffersSalaryRead)
	salaries := map[string]*Salary{}
	if seeSalary {
		srows, err := store.New(tx).OfferSalaries(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range srows {
			amount, err := database.DecimalString(r.Amount)
			if err != nil {
				return nil, err
			}
			salaries[r.OfferID] = &Salary{Amount: *amount, Currency: r.Currency, Period: r.Period}
		}
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		out[i] = View{
			ID: r.ID, ApplicationID: r.ApplicationID, Version: int(r.Version), App: apps[r.ApplicationID], Offer: offerOf(r),
			Salary: salaries[r.ID], SalaryHidden: !seeSalary, Names: names, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}
