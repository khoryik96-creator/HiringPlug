package emails

// WhatsApp messages: the company's templates, and a message filled for a
// candidate or a contact with the numbers to open the chat with. The message
// is sent from the recruiter's own WhatsApp (a wa.me chat link); the web logs
// it as a note.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const whatsAppNameIndex = "whatsapp_template_name_idx"

// WhatsAppTemplates returns the company's WhatsApp templates, adding the
// starters the first time.
func (s *Service) WhatsAppTemplates(ctx context.Context, p authz.Principal) ([]store.WhatsappTemplate, error) {
	var out []store.WhatsappTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListWhatsAppTemplates(ctx)
		if err != nil || len(rows) > 0 {
			out = rows
			return err
		}
		for _, t := range WhatsAppStarters {
			if err := q.SeedWhatsAppTemplate(ctx, store.SeedWhatsAppTemplateParams{Name: t.Name, Body: t.Body}); err != nil {
				return err
			}
		}
		out, err = q.ListWhatsAppTemplates(ctx)
		return err
	})
	return out, err
}

// CreateWhatsAppTemplate adds a WhatsApp template.
func (s *Service) CreateWhatsAppTemplate(ctx context.Context, p authz.Principal, t WhatsAppTemplate) (store.WhatsappTemplate, error) {
	t, errs := NormalizeWhatsAppTemplate(t)
	if err := errs.Err(); err != nil {
		return store.WhatsappTemplate{}, err
	}
	var out store.WhatsappTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).InsertWhatsAppTemplate(ctx, store.InsertWhatsAppTemplateParams{Name: t.Name, Body: t.Body})
		if database.IsUniqueViolation(err, whatsAppNameIndex) {
			return nameTaken()
		}
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{Action: "whatsapp_template.created", EntityType: "whatsapp_template", EntityID: row.ID, Changes: map[string]any{"name": t.Name}})
	})
	return out, err
}

// UpdateWhatsAppTemplate changes a WhatsApp template, if nobody changed it
// since version.
func (s *Service) UpdateWhatsAppTemplate(ctx context.Context, p authz.Principal, id string, version int, t WhatsAppTemplate) (store.WhatsappTemplate, error) {
	t, errs := NormalizeWhatsAppTemplate(t)
	if err := errs.Err(); err != nil {
		return store.WhatsappTemplate{}, err
	}
	var out store.WhatsappTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		old, err := q.GetWhatsAppTemplate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("WhatsApp template")
		}
		if err != nil {
			return err
		}
		row, err := q.UpdateWhatsAppTemplate(ctx, store.UpdateWhatsAppTemplateParams{
			ID: id, ExpectedVersion: int32(version), Name: t.Name, Body: t.Body, //nolint:gosec // versions are small
		})
		switch {
		case database.IsNoRows(err):
			return apperr.StaleVersion("WhatsApp template")
		case database.IsUniqueViolation(err, whatsAppNameIndex):
			return nameTaken()
		case err != nil:
			return err
		}
		out = row
		changes := map[string]any{}
		if old.Name != row.Name {
			changes["name"] = map[string]string{"from": old.Name, "to": row.Name}
		}
		if old.Body != row.Body {
			changes["body"] = "changed"
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "whatsapp_template.updated", EntityType: "whatsapp_template", EntityID: id, Changes: changes})
	})
	return out, err
}

// DeleteWhatsAppTemplate deletes a WhatsApp template.
func (s *Service) DeleteWhatsAppTemplate(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		old, err := q.GetWhatsAppTemplate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("WhatsApp template")
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteWhatsAppTemplate(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "whatsapp_template.deleted", EntityType: "whatsapp_template", EntityID: id, Changes: map[string]any{"name": old.Name}})
	})
}

// WhatsAppDraftRequest is what to fill a WhatsApp message for.
type WhatsAppDraftRequest struct {
	TemplateID             *string
	CandidateID, ContactID *string // one of them
	ApplicationID          *string
	TimeZone               string
}

// WhatsAppNumber is one of the person's numbers; Chat is empty when it is
// not a valid number.
type WhatsAppNumber struct{ Label, Number, Chat string }

// WhatsAppDraft is a filled message and the numbers to send it to.
type WhatsAppDraft struct {
	Text    string
	Missing []string
	Numbers []WhatsAppNumber
	Fields  []WhatsAppField // every field a template may use, with the person's value
}

// WhatsAppDraft fills a template (or starts blank) for a candidate or a
// contact, salary aside.
func (s *Service) WhatsAppDraft(ctx context.Context, p authz.Principal, r WhatsAppDraftRequest) (WhatsAppDraft, error) {
	loc, err := time.LoadLocation(r.TimeZone)
	if err != nil || r.TimeZone == "Local" {
		var errs apperr.FieldErrors
		errs.Add("timeZone", "is not a time zone, e.g. Asia/Kuala_Lumpur")
		return WhatsAppDraft{}, errs.Err()
	}
	out := WhatsAppDraft{Missing: []string{}, Numbers: []WhatsAppNumber{}}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var body string
		if r.TemplateID != nil {
			tpl, err := store.New(tx).GetWhatsAppTemplate(ctx, *r.TemplateID)
			if database.IsNoRows(err) {
				return apperr.NotFound("WhatsApp template")
			}
			if err != nil {
				return err
			}
			body = tpl.Body
		}
		var values map[string]string
		var phones [][2]string // label, number
		switch {
		case r.CandidateID != nil:
			dc, err := s.context(ctx, tx, p, *r.CandidateID, r.ApplicationID, nil)
			if err != nil {
				return err
			}
			dc.Location = loc
			values = Values(dc)
			phones = append(phones, [2]string{"Phone", deref(dc.Candidate.Phone)})
		case r.ContactID != nil:
			ct, err := s.deps.Contacts.Contact(ctx, tx, *r.ContactID)
			if err != nil {
				return err
			}
			tenant, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
			if err != nil {
				return err
			}
			values = ContactValues(ct, p.Name, tenant)
			phones = append(phones, [2]string{"Mobile", deref(ct.Mobile)}, [2]string{"Office phone", deref(ct.Phone)})
		default:
			var errs apperr.FieldErrors
			errs.Add("candidateId", "give candidateId or contactId")
			return errs.Err()
		}
		out.Fields = WhatsAppFields(values)
		if text, missing := Fill(body, WithoutSalary(values)); missing != nil {
			out.Text, out.Missing = text, missing
		} else {
			out.Text = text
		}
		for _, ph := range phones {
			if ph[1] == "" {
				continue
			}
			chat, _ := ChatNumber(ph[1], s.deps.PhoneRegion)
			out.Numbers = append(out.Numbers, WhatsAppNumber{Label: ph[0], Number: ph[1], Chat: chat})
		}
		return nil
	})
	return out, err
}
