package emails

// Banners in Outlook drafts: a banner the person inserts (or that a template
// or their signature holds) is attached to the draft as an inline picture
// (its content id is the banner's id), so the email shows it wherever it is
// read. A banner taken out of the message leaves the draft too. Only the
// company's own live banners are attached: a banner quoted from an earlier
// message (deleted since, or another company's) is left as it is, so a reply
// or forward is never refused for its history.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// draftBannerPrefix names a banner's file in a draft: hp-banner-<banner id>.<ext>.
const draftBannerPrefix = "hp-banner-"

var cidBannerSrc = regexp.MustCompile(`\s?src="cid:ebn_[0-9a-hjkmnp-tv-z]{26}"`)

// draftBodyHTML is a draft's message as it is written to Outlook: made safe,
// each banner pointing at its picture in the draft, the field marks gone.
func draftBodyHTML(html string) string {
	clean := sanitizeOutlookHTML(html)
	return WithoutFieldMarks(WithBannerCIDs(cidBannerSrc.ReplaceAllString(clean, "")))
}

// syncDraftBanners attaches the banners html shows that the draft lacks,
// and removes those it no longer shows; it reports whether it changed the
// draft (its version then changes).
func (s *Service) syncDraftBanners(ctx context.Context, q *store.Queries, token, target, html string) (bool, error) {
	ids := BannerIDs(html)
	var page outlookPage[outlookAttachment]
	if err := s.outlookRequest(ctx, token, http.MethodGet, target+"/attachments?$select=id,name&$top=100", nil, &page); err != nil {
		return false, err
	}
	have := map[string]string{} // banner id -> the draft's attachment id
	if page.Value != nil {
		for _, a := range *page.Value {
			if strings.HasPrefix(a.Name, draftBannerPrefix) {
				have[strings.TrimSuffix(strings.TrimPrefix(a.Name, draftBannerPrefix), path.Ext(a.Name))] = a.ID
			}
		}
	}
	changed := false
	var missing []string
	for _, id := range ids {
		if _, ok := have[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		rows, err := q.GetBanners(ctx, missing)
		if err != nil {
			return false, err
		}
		for _, b := range rows {
			if b.DeletedAt != nil {
				continue
			}
			f, err := s.files.Open(b.StorageKey)
			if err != nil {
				return changed, err
			}
			data, err := io.ReadAll(io.LimitReader(f, maxBannerBytes+1))
			_ = f.Close()
			if err != nil {
				return changed, err
			}
			part := mail.Part{Name: draftBannerPrefix + b.ID + extension(b.ContentType), ContentType: b.ContentType, ContentID: b.ID, Data: data}
			if err := s.putMicrosoftAttachment(ctx, s.endpoint(ProviderMicrosoft), token, target, part, true); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	for id, attachment := range have {
		if slices.Contains(ids, id) {
			continue
		}
		err := s.outlookRequest(ctx, token, http.MethodDelete, target+"/attachments/"+url.PathEscape(attachment), nil, nil)
		var graph *outlookGraphError
		if err != nil && (!errors.As(err, &graph) || graph.status != http.StatusNotFound) {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// withDraftBanners brings a draft just written in line with its banners,
// and returns it as it now is.
func (s *Service) withDraftBanners(ctx context.Context, q *store.Queries, token string, remote outlookRemoteMessage, in api.OutlookDraftInput) (outlookRemoteMessage, error) {
	if in.BodyHtml == nil {
		return remote, nil
	}
	target := s.outlookRoot() + "/messages/" + url.PathEscape(remote.ID)
	changed, err := s.syncDraftBanners(ctx, q, token, target, draftBodyHTML(*in.BodyHtml))
	if err != nil || !changed {
		return remote, err
	}
	return s.readRemoteDraft(ctx, token, remote.ID)
}
