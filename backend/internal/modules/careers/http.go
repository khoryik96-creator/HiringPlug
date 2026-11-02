package careers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

// HTTP implements the careers operations of the contract.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// GetCareersPage handles GET /v1/public/careers/{slug}.
func (h *HTTP) GetCareersPage(ctx context.Context, req api.GetCareersPageRequestObject) (api.GetCareersPageResponseObject, error) {
	name, ads, err := h.svc.Page(ctx, req.Slug)
	if err != nil {
		return nil, err
	}
	out := api.GetCareersPage200JSONResponse{CompanyName: name, Ads: make([]api.PublicJobAd, len(ads))}
	for i, a := range ads {
		out.Ads[i] = adToAPI(a)
	}
	return out, nil
}

// GetCareersFeed handles GET /v1/public/careers/{slug}/jobs.xml.
func (h *HTTP) GetCareersFeed(ctx context.Context, req api.GetCareersFeedRequestObject) (api.GetCareersFeedResponseObject, error) {
	out, err := h.svc.Feed(ctx, req.Slug)
	if err != nil {
		return nil, err
	}
	return api.GetCareersFeed200ApplicationxmlResponse{Body: bytes.NewReader(out), ContentLength: int64(len(out))}, nil
}

// GetCareersAd handles GET /v1/public/careers/{slug}/ads/{adId}.
func (h *HTTP) GetCareersAd(ctx context.Context, req api.GetCareersAdRequestObject) (api.GetCareersAdResponseObject, error) {
	ad, err := h.svc.Ad(ctx, req.Slug, req.AdId)
	if err != nil {
		return nil, err
	}
	return api.GetCareersAd200JSONResponse(adToAPI(ad)), nil
}

// ApplyToJobAd handles POST /v1/public/careers/{slug}/ads/{adId}/applications
// (multipart/form-data, fields in any order).
func (h *HTTP) ApplyToJobAd(ctx context.Context, req api.ApplyToJobAdRequestObject) (api.ApplyToJobAdResponseObject, error) {
	var form Form
	var fileName string
	var cv []byte
	text := map[string]*string{
		"firstName": &form.FirstName, "lastName": &form.LastName, "email": &form.Email,
		"phone": &form.Phone, "consent": &form.Consent, "website": &form.Website, "via": &form.Via,
	}
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, readError(err)
		}
		name := part.FormName()
		switch {
		case name == "file":
			fileName = part.FileName()
			if cv, err = io.ReadAll(io.LimitReader(part, h.maxUploadBytes+1)); err != nil {
				return nil, readError(err)
			}
			if int64(len(cv)) > h.maxUploadBytes {
				return nil, apperr.New(api.ErrorCodePAYLOADTOOLARGE, fmt.Sprintf("CVs can be at most %d MB.", h.maxUploadBytes>>20))
			}
		case text[name] != nil:
			b, err := io.ReadAll(io.LimitReader(part, 1024))
			if err != nil {
				return nil, readError(err)
			}
			*text[name] = string(b)
		}
	}
	if len(cv) == 0 {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "file", Message: "attach your CV (PDF or Word)"}})
	}
	if err := h.svc.Apply(ctx, req.Slug, req.AdId, clientip.From(ctx), form, fileName, cv); err != nil {
		return nil, err
	}
	return api.ApplyToJobAd202JSONResponse{Received: true}, nil
}

func readError(err error) error {
	// Too-large bodies surface as *http.MaxBytesError; WriteError maps that to 413.
	if strings.Contains(err.Error(), "request body too large") {
		return err
	}
	return apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "is not a valid multipart/form-data form"}})
}

func adToAPI(a Ad) api.PublicJobAd {
	out := api.PublicJobAd{
		Id: a.ID, Title: a.Title, Description: a.Description, PublishedAt: a.PublishedAt,
		Location: c.OrNull(a.Location), SalaryText: c.OrNull(a.SalaryText),
		WorkType:        c.OrNull(enumPtr[api.WorkType](a.WorkType)),
		WorkArrangement: c.OrNull(enumPtr[api.WorkArrangement](a.WorkArrangement)),
	}
	var closes *openapi_types.Date
	if a.ClosesOn != nil {
		closes = &openapi_types.Date{Time: *a.ClosesOn}
	}
	out.ClosesOn = c.OrNull(closes)
	return out
}

func enumPtr[T ~string](s *string) *T {
	if s == nil {
		return nil
	}
	v := T(*s)
	return &v
}
