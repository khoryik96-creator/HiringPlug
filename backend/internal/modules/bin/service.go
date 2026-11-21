package bin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/bin/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the bin's use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
}

// Check is what deleting a record would take with it.
type Check struct {
	Label                       string
	Contacts, Jobs, Application int
	Blocked                     string
}

// linked returns a record's label, a company's contacts and jobs, and the
// applications hidden with it (in a transaction that does not see the Bin:
// a binned record is not found).
func (s *Service) linked(ctx context.Context, tx pgx.Tx, kind, id string) (label string, contacts, jobs []string, apps []Application, err error) {
	d := s.deps
	switch kind {
	case "document":
		label, err = d.Files.DocumentLabel(ctx, tx, id)
	case "candidate":
		if label, err = d.Candidates.Label(ctx, tx, id); err == nil {
			apps, err = d.Pipeline.ApplicationsOf(ctx, tx, []string{id}, nil)
		}
	case "contact":
		label, err = d.Contacts.Label(ctx, tx, id)
	case "company":
		if label, err = d.Companies.Label(ctx, tx, id); err != nil {
			return
		}
		if contacts, err = d.Contacts.IDsOfCompany(ctx, tx, id); err != nil {
			return
		}
		if jobs, err = d.Jobs.IDsOfCompany(ctx, tx, id); err == nil && len(jobs) > 0 {
			apps, err = d.Pipeline.ApplicationsOf(ctx, tx, nil, jobs)
		}
	default:
		err = apperr.NotFound(kind)
	}
	return
}

// Check says what deleting a record would take with it, and whether it may be deleted.
func (s *Service) Check(ctx context.Context, p authz.Principal, kind, id string) (Check, error) {
	var out Check
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		label, contacts, jobs, apps, err := s.linked(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		out = Check{Label: label, Contacts: len(contacts), Jobs: len(jobs), Application: len(apps)}
		if placed, err := s.deps.Pipeline.Placed(ctx, tx, appIDs(apps)); err != nil {
			return err
		} else if placed > 0 {
			out.Blocked = placedReason(placed)
		}
		return nil
	})
	return out, err
}

// Bin deletes a record to the Bin. A company's contacts and jobs move to
// moveTo when it is given (another company), and otherwise go with it.
func (s *Service) Bin(ctx context.Context, p authz.Principal, kind, id, moveTo string) (store.BinEntry, error) {
	var entry store.BinEntry
	d := s.deps
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		label, contacts, jobs, apps, err := s.linked(ctx, tx, kind, id)
		if err != nil {
			return err
		}
		var went WentWith
		if kind == "company" && moveTo != "" {
			if moveTo == id {
				var errs apperr.FieldErrors
				errs.Add("moveTo", "choose another company")
				return errs.Err()
			}
			if _, err := d.Companies.Label(ctx, tx, moveTo); err != nil {
				var errs apperr.FieldErrors
				errs.Add("moveTo", "no such company")
				return errs.Err()
			}
			if err := d.Contacts.MoveToCompany(ctx, tx, id, moveTo); err != nil {
				return err
			}
			if err := d.Jobs.MoveToCompany(ctx, tx, id, moveTo); err != nil {
				return err
			}
			apps = nil // they moved with their jobs
		} else if kind == "company" {
			went = WentWith{Contacts: contacts, Jobs: jobs}
		}
		if placed, err := d.Pipeline.Placed(ctx, tx, appIDs(apps)); err != nil {
			return err
		} else if placed > 0 {
			return apperr.New(api.ErrorCodeBINBLOCKED, placedReason(placed))
		}
		q := store.New(tx)
		if err := q.ShowBinned(ctx); err != nil {
			return err
		}
		switch kind {
		case "document":
			err = d.Files.BinActiveDocument(ctx, tx, id)
		case "candidate":
			_, err = d.Candidates.Bin(ctx, tx, id)
		case "contact":
			_, err = d.Contacts.Bin(ctx, tx, []string{id})
		case "company":
			if _, err = d.Companies.Bin(ctx, tx, id); err == nil && len(went.Contacts) > 0 {
				_, err = d.Contacts.Bin(ctx, tx, went.Contacts)
			}
			if err == nil && len(went.Jobs) > 0 {
				err = d.Jobs.Bin(ctx, tx, went.Jobs)
			}
		}
		if err != nil {
			return err
		}
		if err := d.Pipeline.SetBinned(ctx, tx, appIDs(apps), true); err != nil {
			return err
		}
		entry, err = q.InsertBinEntry(ctx, store.InsertBinEntryParams{
			Kind: kind, RecordID: id, Label: label, WentWith: went.encode(), BinnedName: p.Name, PurgeAfter: time.Now().Add(Keep),
		})
		if err != nil {
			return err
		}
		if _, err := jobqueue.EnqueueAt(ctx, tx, p.TenantID, api.JobKindBinPurge, api.BinPurgeJob{EntryId: entry.ID}, entry.PurgeAfter); err != nil {
			return err
		}
		changes := map[string]any{"contacts": len(went.Contacts), "jobs": len(went.Jobs), "applications": len(apps)}
		addLabel(changes, kind, label)
		if moveTo != "" {
			changes["movedTo"] = moveTo
		}
		return audit.Record(ctx, tx, audit.Entry{Action: kind + ".binned", EntityType: kind, EntityID: id, Changes: changes})
	})
	return entry, err
}

// List returns what is in the Bin, the latest first.
func (s *Service) List(ctx context.Context, p authz.Principal) ([]store.BinEntry, error) {
	var out []store.BinEntry
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) (err error) {
		out, err = store.New(tx).ListBinEntries(ctx)
		return err
	})
	return out, err
}

// Restore brings a record back from the Bin, with what went with it.
func (s *Service) Restore(ctx context.Context, p authz.Principal, entryID string) error {
	d := s.deps
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.ShowBinned(ctx); err != nil {
			return err
		}
		e, err := q.GetBinEntry(ctx, entryID)
		if database.IsNoRows(err) {
			return apperr.NotFound("bin entry")
		}
		if err != nil {
			return err
		}
		went := decodeWentWith(e.WentWith)
		var apps []Application
		switch e.Kind {
		case "document":
			if !time.Now().Before(e.PurgeAfter) {
				return apperr.NotFound("unexpired bin entry")
			}
			err = d.Files.RestoreDocument(ctx, tx, e.RecordID)
		case "candidate":
			err = d.Candidates.Restore(ctx, tx, e.RecordID)
			if err == nil {
				apps, err = d.Pipeline.ApplicationsOf(ctx, tx, []string{e.RecordID}, nil)
			}
		case "contact":
			err = d.Contacts.Restore(ctx, tx, []string{e.RecordID})
		case "company":
			err = d.Companies.Restore(ctx, tx, e.RecordID)
			if err == nil && len(went.Contacts) > 0 {
				err = d.Contacts.Restore(ctx, tx, went.Contacts)
			}
			if err == nil && len(went.Jobs) > 0 {
				if err = d.Jobs.Restore(ctx, tx, went.Jobs); err == nil {
					apps, err = d.Pipeline.ApplicationsOf(ctx, tx, nil, went.Jobs)
				}
			}
		}
		if d.Taken != nil && d.Taken(err) {
			return apperr.New(api.ErrorCodeBINRESTORECONFLICT,
				"Another record has this one's email or name now. Change one of them, then restore.")
		}
		if err != nil {
			return err
		}
		if err := s.showApplications(ctx, tx, apps); err != nil {
			return err
		}
		if _, err := q.DeleteBinEntry(ctx, e.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: e.Kind + ".restored", EntityType: e.Kind, EntityID: e.RecordID,
			Changes: addLabel(map[string]any{"contacts": len(went.Contacts), "jobs": len(went.Jobs)}, e.Kind, e.Label)})
	})
}

// showApplications shows again the applications whose candidate and job are both out of the Bin.
func (s *Service) showApplications(ctx context.Context, tx pgx.Tx, apps []Application) error {
	if len(apps) == 0 {
		return nil
	}
	var cands, jobs []string
	for _, a := range apps {
		cands, jobs = append(cands, a.CandidateID), append(jobs, a.JobID)
	}
	binnedC, err := s.deps.Candidates.Binned(ctx, tx, cands)
	if err != nil {
		return err
	}
	binnedJ, err := s.deps.Jobs.Binned(ctx, tx, jobs)
	if err != nil {
		return err
	}
	return s.deps.Pipeline.SetBinned(ctx, tx, unbinnable(apps, binnedC, binnedJ), false)
}

// addLabel adds a record's name to an audit entry's changes, for a company
// only: a candidate's or contact's name is a person's, and the audit log is
// kept after they are deleted for good (the Bin's own list names them).
func addLabel(changes map[string]any, kind, label string) map[string]any {
	if kind == "company" {
		changes["label"] = label
	}
	return changes
}
