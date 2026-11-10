package emails

import (
	"context"
	"errors"
	"io"
	"mime/multipart"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the emails operations of the contract.
type HTTP struct{ svc *Service }

// ListEmailTemplates handles GET /v1/email-templates.
func (h *HTTP) ListEmailTemplates(ctx context.Context, _ api.ListEmailTemplatesRequestObject) (api.ListEmailTemplatesResponseObject, error) {
	rows, err := h.svc.Templates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListEmailTemplates200JSONResponse{Items: make([]api.EmailTemplate, len(rows)), Placeholders: Placeholders, Fields: make([]api.EmailField, len(Fields))}
	for i, f := range Fields {
		out.Fields[i] = api.EmailField{Token: f.Token(), Key: f.Key, Label: f.Label, Salary: f.Salary}
	}
	for i, r := range rows {
		out.Items[i] = templateToAPI(r)
	}
	return out, nil
}

// CreateEmailTemplate handles POST /v1/email-templates.
func (h *HTTP) CreateEmailTemplate(ctx context.Context, req api.CreateEmailTemplateRequestObject) (api.CreateEmailTemplateResponseObject, error) {
	b := req.Body
	row, err := h.svc.CreateTemplate(ctx, authz.MustFromContext(ctx), Template{Name: b.Name, Subject: b.Subject, Body: deref(b.Body), BodyHTML: deref(b.BodyHtml)})
	if err != nil {
		return nil, err
	}
	return api.CreateEmailTemplate201JSONResponse(templateToAPI(row)), nil
}

// UpdateEmailTemplate handles PUT /v1/email-templates/{templateId}.
func (h *HTTP) UpdateEmailTemplate(ctx context.Context, req api.UpdateEmailTemplateRequestObject) (api.UpdateEmailTemplateResponseObject, error) {
	b := req.Body
	row, err := h.svc.UpdateTemplate(ctx, authz.MustFromContext(ctx), req.TemplateId, b.Version, Template{Name: b.Name, Subject: b.Subject, Body: deref(b.Body), BodyHTML: deref(b.BodyHtml)})
	if err != nil {
		return nil, err
	}
	return api.UpdateEmailTemplate200JSONResponse(templateToAPI(row)), nil
}

// DraftEmail handles POST /v1/email-drafts.
func (h *HTTP) DraftEmail(ctx context.Context, req api.DraftEmailRequestObject) (api.DraftEmailResponseObject, error) {
	b := req.Body
	d, err := h.svc.Draft(ctx, authz.MustFromContext(ctx), DraftRequest{
		TemplateID: b.TemplateId, CandidateID: b.CandidateId, ContactID: b.ContactId, ApplicationID: b.ApplicationId, InterviewID: b.InterviewId, TimeZone: b.TimeZone,
	})
	if err != nil {
		return nil, err
	}
	return api.DraftEmail200JSONResponse{To: convert[openapi_types.Email](d.To), Subject: d.Subject, Body: d.Body, BodyHtml: c.OrNull(d.BodyHTML), Missing: d.Missing}, nil
}

// ListEmails handles GET /v1/emails.
func (h *HTTP) ListEmails(ctx context.Context, req api.ListEmailsRequestObject) (api.ListEmailsResponseObject, error) {
	rows, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.CandidateId, req.Params.ContactId)
	if err != nil {
		return nil, err
	}
	out := api.ListEmails200JSONResponse{Items: make([]api.Email, len(rows))}
	for i, r := range rows {
		out.Items[i] = emailToAPI(r)
	}
	return out, nil
}

// SendEmail handles POST /v1/emails.
func (h *HTTP) SendEmail(ctx context.Context, req api.SendEmailRequestObject) (api.SendEmailResponseObject, error) {
	b := req.Body
	r := SendRequest{
		Outgoing:    Outgoing{To: convert[string](b.To), Subject: b.Subject, Body: deref(b.Body), BodyHTML: deref(b.BodyHtml)},
		CandidateID: b.CandidateId, ApplicationID: b.ApplicationId, ContactID: b.ContactId, TimeZone: deref(b.TimeZone),
	}
	if b.Cc != nil {
		r.Cc = convert[string](*b.Cc)
	}
	if b.Bcc != nil {
		r.Bcc = convert[string](*b.Bcc)
	}
	if b.AttachmentIds != nil {
		r.AttachmentIDs = *b.AttachmentIds
	}
	if b.Cvs != nil {
		for _, cv := range *b.Cvs {
			r.CVs = append(r.CVs, CVRef{DocumentID: cv.DocumentId, AsPDF: deref(cv.AsPdf)})
		}
	}
	r.Candidates = shortlistFrom(b.Candidates)
	row, err := h.svc.Send(ctx, authz.MustFromContext(ctx), r)
	if err != nil {
		return nil, err
	}
	return api.SendEmail202JSONResponse(emailToAPI(row)), nil
}

// PreviewEmail handles POST /v1/email-previews.
func (h *HTTP) PreviewEmail(ctx context.Context, req api.PreviewEmailRequestObject) (api.PreviewEmailResponseObject, error) {
	b := req.Body
	p, err := h.svc.Preview(ctx, authz.MustFromContext(ctx), b.Subject, b.BodyHtml, b.CandidateId, b.ApplicationId, b.InterviewId, b.ContactId, shortlistFrom(b.Candidates), b.TimeZone)
	if err != nil {
		return nil, err
	}
	return api.PreviewEmail200JSONResponse{Subject: p.Subject, BodyHtml: p.BodyHTML, Missing: nonNil(p.Missing), Salary: p.Salary, ClientEmail: c.OrNull(p.ClientEmail), FieldValues: p.FieldValues}, nil
}

// DeleteEmailTemplate handles DELETE /v1/email-templates/{templateId}.
func (h *HTTP) DeleteEmailTemplate(ctx context.Context, req api.DeleteEmailTemplateRequestObject) (api.DeleteEmailTemplateResponseObject, error) {
	if err := h.svc.DeleteTemplate(ctx, authz.MustFromContext(ctx), req.TemplateId); err != nil {
		return nil, err
	}
	return api.DeleteEmailTemplate204Response{}, nil
}

// AddEmailAttachment handles POST /v1/email-attachments.
func (h *HTTP) AddEmailAttachment(ctx context.Context, req api.AddEmailAttachmentRequestObject) (api.AddEmailAttachmentResponseObject, error) {
	form, err := readForm(req.Body, maxFileBytes, "A file can be at most 10 MB.")
	if err != nil {
		return nil, err
	}
	row, err := h.svc.AddAttachment(ctx, authz.MustFromContext(ctx), form.fileName, form.data)
	if err != nil {
		return nil, err
	}
	return api.AddEmailAttachment201JSONResponse(attachmentToAPI(row)), nil
}

// ListEmailBanners handles GET /v1/email-banners.
func (h *HTTP) ListEmailBanners(ctx context.Context, _ api.ListEmailBannersRequestObject) (api.ListEmailBannersResponseObject, error) {
	rows, err := h.svc.Banners(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListEmailBanners200JSONResponse{Items: make([]api.EmailBanner, len(rows))}
	for i, r := range rows {
		out.Items[i] = bannerToAPI(r)
	}
	return out, nil
}

// AddEmailBanner handles POST /v1/email-banners.
func (h *HTTP) AddEmailBanner(ctx context.Context, req api.AddEmailBannerRequestObject) (api.AddEmailBannerResponseObject, error) {
	form, err := readForm(req.Body, maxBannerBytes, "A banner can be at most 1 MB.")
	if err != nil {
		return nil, err
	}
	row, err := h.svc.AddBanner(ctx, authz.MustFromContext(ctx), form.name, form.fileName, form.data)
	if err != nil {
		return nil, err
	}
	return api.AddEmailBanner201JSONResponse(bannerToAPI(row)), nil
}

// DeleteEmailBanner handles DELETE /v1/email-banners/{bannerId}.
func (h *HTTP) DeleteEmailBanner(ctx context.Context, req api.DeleteEmailBannerRequestObject) (api.DeleteEmailBannerResponseObject, error) {
	if err := h.svc.DeleteBanner(ctx, authz.MustFromContext(ctx), req.BannerId); err != nil {
		return nil, err
	}
	return api.DeleteEmailBanner204Response{}, nil
}

// GetEmailBannerFile handles GET /v1/email-banners/{bannerId}/file.
func (h *HTTP) GetEmailBannerFile(ctx context.Context, req api.GetEmailBannerFileRequestObject) (api.GetEmailBannerFileResponseObject, error) {
	_, f, err := h.svc.BannerFile(ctx, authz.MustFromContext(ctx), req.BannerId)
	if err != nil {
		return nil, err
	}
	return api.GetEmailBannerFile200ApplicationoctetStreamResponse{Body: f}, nil
}

type form struct {
	name, fileName string
	data           []byte
}

// readForm reads a multipart form with a "file" (at most limit bytes) and an
// optional "name".
func readForm(r *multipart.Reader, limit int64, tooLarge string) (form, error) {
	var out form
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data form"}})
		}
		switch part.FormName() {
		case "file":
			out.fileName = part.FileName()
			if out.data, err = io.ReadAll(io.LimitReader(part, limit+1)); err != nil {
				return out, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "could not be read"}})
			}
			if int64(len(out.data)) > limit {
				return out, apperr.New(api.ErrorCodePAYLOADTOOLARGE, tooLarge)
			}
		case "name":
			v, _ := io.ReadAll(io.LimitReader(part, 400))
			out.name = string(v)
		default:
			return out, apperr.Validation(apperr.FieldErrors{{Field: part.FormName(), Message: "unexpected field"}})
		}
	}
	if len(out.data) == 0 {
		return out, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "is required and must not be empty"}})
	}
	return out, nil
}

func attachmentToAPI(r store.EmailAttachment) api.EmailAttachment {
	return api.EmailAttachment{Id: r.ID, FileName: r.FileName, ContentType: r.ContentType, SizeBytes: int(r.SizeBytes)}
}

func bannerToAPI(r store.EmailBanner) api.EmailBanner {
	return api.EmailBanner{Id: r.ID, Name: r.Name, ContentType: r.ContentType, SizeBytes: int(r.SizeBytes), CreatedAt: r.CreatedAt}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func templateToAPI(r store.EmailTemplate) api.EmailTemplate {
	return api.EmailTemplate{
		Id: r.ID, Version: int(r.Version), Name: r.Name, Subject: r.Subject, Body: r.Body, BodyHtml: c.OrNull(r.BodyHtml),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func emailToAPI(v EmailView) api.Email {
	r := v.Email
	files := make([]api.EmailAttachment, len(v.Attachments))
	for i, f := range v.Attachments {
		files[i] = attachmentToAPI(f)
	}
	return api.Email{
		Id: r.ID, CandidateId: c.OrNull(r.CandidateID), ApplicationId: c.OrNull(r.ApplicationID), ContactId: c.OrNull(r.ContactID),
		To: r.ToAddresses, Cc: r.CcAddresses, Bcc: r.BccAddresses, Subject: r.Subject, Body: r.Body, BodyHtml: c.OrNull(r.BodyHtml),
		Attachments: files, Salary: r.Salary, Status: api.EmailStatus(r.Status), Error: c.OrNull(r.Error),
		SentBy: r.SentBy, SentByName: r.SenderName, CreatedAt: r.CreatedAt, SentAt: c.OrNull(r.SentAt),
	}
}

// convert changes a list of strings between string types (addresses).
func convert[To ~string, From ~string](in []From) []To {
	out := make([]To, len(in))
	for i, v := range in {
		out[i] = To(v)
	}
	return out
}

func shortlistFrom(list *[]api.EmailCandidate) []Shortlisted {
	if list == nil {
		return nil
	}
	out := make([]Shortlisted, len(*list))
	for i, c := range *list {
		out[i] = Shortlisted{CandidateID: c.CandidateId, ApplicationID: c.ApplicationId}
	}
	return out
}

// SendEmailsSeparately handles POST /v1/email-batches.
func (h *HTTP) SendEmailsSeparately(ctx context.Context, req api.SendEmailsSeparatelyRequestObject) (api.SendEmailsSeparatelyResponseObject, error) {
	b := req.Body
	r := BatchRequest{Subject: b.Subject, Body: deref(b.Body), BodyHTML: deref(b.BodyHtml), TimeZone: deref(b.TimeZone), CheckOnly: deref(b.CheckOnly)}
	for _, x := range b.Recipients {
		r.Recipients = append(r.Recipients, Recipient{CandidateID: x.CandidateId, ContactID: x.ContactId})
	}
	if b.AttachmentIds != nil {
		r.AttachmentIDs = *b.AttachmentIds
	}
	res, err := h.svc.SendSeparately(ctx, authz.MustFromContext(ctx), r)
	if err != nil {
		return nil, err
	}
	out := api.SendEmailsSeparately200JSONResponse{Sent: res.Sent, Recipients: make([]api.EmailBatchRecipient, len(res.Recipients))}
	for i, x := range res.Recipients {
		out.Recipients[i] = api.EmailBatchRecipient{
			CandidateId: c.OrNull(x.CandidateID), ContactId: c.OrNull(x.ContactID), Name: x.Name,
			To: c.OrNull(x.To), EmailId: c.OrNull(x.EmailID), Problem: c.OrNull(x.Problem),
		}
	}
	return out, nil
}
