package shortlists

import (
	"bytes"
	"context"
	"mime"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

// HTTP implements the shortlists operations of the contract.
type HTTP struct{ svc *Service }

// ListClientShortlists handles GET /v1/jobs/{jobId}/client-shortlists.
func (h *HTTP) ListClientShortlists(ctx context.Context, req api.ListClientShortlistsRequestObject) (api.ListClientShortlistsResponseObject, error) {
	items, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	out := api.ListClientShortlists200JSONResponse{Items: make([]api.ClientShortlist, len(items))}
	for i, s := range items {
		out.Items[i] = toAPI(s)
	}
	return out, nil
}

// CreateClientShortlist handles POST /v1/jobs/{jobId}/client-shortlists.
func (h *HTTP) CreateClientShortlist(ctx context.Context, req api.CreateClientShortlistRequestObject) (api.CreateClientShortlistResponseObject, error) {
	b := req.Body
	in := Input{ExpiresInDays: b.ExpiresInDays, Message: b.Message, EmailHiringContact: b.EmailHiringContact != nil && *b.EmailHiringContact}
	for _, p := range b.Candidates {
		in.Picks = append(in.Picks, Pick{ApplicationID: p.ApplicationId, DocumentID: p.DocumentId, Note: p.Note})
	}
	s, url, err := h.svc.Create(ctx, authz.MustFromContext(ctx), req.JobId, in)
	if err != nil {
		return nil, err
	}
	return api.CreateClientShortlist201JSONResponse{Shortlist: toAPI(s), Url: url}, nil
}

// RevokeClientShortlist handles POST /v1/client-shortlists/{shortlistId}/revoke.
func (h *HTTP) RevokeClientShortlist(ctx context.Context, req api.RevokeClientShortlistRequestObject) (api.RevokeClientShortlistResponseObject, error) {
	s, err := h.svc.Revoke(ctx, authz.MustFromContext(ctx), req.ShortlistId)
	if err != nil {
		return nil, err
	}
	return api.RevokeClientShortlist200JSONResponse(toAPI(s)), nil
}

// GetPublicClientShortlist handles GET /v1/public/client-shortlists/{shortlistToken}.
func (h *HTTP) GetPublicClientShortlist(ctx context.Context, req api.GetPublicClientShortlistRequestObject) (api.GetPublicClientShortlistResponseObject, error) {
	p, err := h.svc.PublicPage(ctx, req.ShortlistToken)
	if err != nil {
		return nil, err
	}
	return api.GetPublicClientShortlist200JSONResponse(pageToAPI(p)), nil
}

// AnswerPublicClientShortlist handles POST .../candidates/{shortlistCandidateId}/answer.
func (h *HTTP) AnswerPublicClientShortlist(ctx context.Context, req api.AnswerPublicClientShortlistRequestObject) (api.AnswerPublicClientShortlistResponseObject, error) {
	p, err := h.svc.Answer(ctx, req.ShortlistToken, req.ShortlistCandidateId, string(req.Body.Decision), req.Body.Comment, clientip.From(ctx))
	if err != nil {
		return nil, err
	}
	return api.AnswerPublicClientShortlist200JSONResponse(pageToAPI(p)), nil
}

// GetPublicClientShortlistCv handles GET .../candidates/{shortlistCandidateId}/cv.
func (h *HTTP) GetPublicClientShortlistCv(ctx context.Context, req api.GetPublicClientShortlistCvRequestObject) (api.GetPublicClientShortlistCvResponseObject, error) {
	f, err := h.svc.CV(ctx, req.ShortlistToken, req.ShortlistCandidateId, clientip.From(ctx))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})
	return api.GetPublicClientShortlistCv200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(f.Data), ContentLength: int64(len(f.Data)),
		Headers: api.GetPublicClientShortlistCv200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func toAPI(s Shortlist) api.ClientShortlist {
	out := api.ClientShortlist{
		Id: s.ID, JobId: s.JobID, Status: api.ClientShortlistStatus(s.Status), Message: c.OrNull(s.Message), ExpiresAt: s.ExpiresAt,
		RevokedAt: c.OrNull(s.RevokedAt), OpenedAt: c.OrNull(s.OpenedAt), LastOpenedAt: c.OrNull(s.LastOpenedAt),
		EmailedTo: c.OrNull(s.EmailedTo), CreatedByName: c.OrNull(s.CreatedByName), CreatedAt: s.CreatedAt,
		Candidates: make([]api.ClientShortlistCandidate, len(s.Candidates)),
	}
	for i, x := range s.Candidates {
		out.Candidates[i] = api.ClientShortlistCandidate{
			Id: x.ID, ApplicationId: x.ApplicationID, CandidateId: x.CandidateID, CandidateName: x.CandidateName, Label: x.Label,
			Blind: x.Blind, DocumentId: x.DocumentID, Note: c.OrNull(x.Note), Comment: c.OrNull(x.Comment), AnsweredAt: c.OrNull(x.AnsweredAt),
			Decision: c.OrNull(c.Convert[api.ClientShortlistDecision](x.Decision)),
		}
	}
	return out
}

func pageToAPI(p Page) api.PublicClientShortlist {
	out := api.PublicClientShortlist{
		AgencyName: p.AgencyName, JobTitle: p.JobTitle, RecruiterName: c.OrNull(p.RecruiterName), Message: c.OrNull(p.Message),
		Status: api.ClientShortlistStatus(p.Status), ExpiresAt: p.ExpiresAt, Candidates: make([]api.PublicShortlistCandidate, len(p.Candidates)),
	}
	for i, x := range p.Candidates {
		name := x.CVFileName
		out.Candidates[i] = api.PublicShortlistCandidate{
			Id: x.ID, Label: x.Label, Blind: x.Blind, Note: c.OrNull(x.Note), CvFileName: c.OrNull(&name),
			Comment: c.OrNull(x.Comment), AnsweredAt: c.OrNull(x.AnsweredAt), Decision: c.OrNull(c.Convert[api.ClientShortlistDecision](x.Decision)),
		}
	}
	return out
}
