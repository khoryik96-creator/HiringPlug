package claims

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
)

// A file put into a drive: its id there (to replace or delete it later)
// and where people open it.
type remoteFile struct{ ID, URL string }

// put uploads a receipt into folders (made where missing) as name, in place
// of a file of that name.
func (s *Storage) put(ctx context.Context, provider, token string, folders []string, name, contentType string, data []byte) (remoteFile, error) {
	switch provider {
	case GoogleDrive:
		return s.putGoogle(ctx, token, folders, name, contentType, data)
	case Dropbox:
		return s.putDropbox(ctx, token, folders, name, data)
	}
	return s.putOneDrive(ctx, token, folders, name, data)
}

// remove deletes a file put earlier; one already gone is not an error.
func (s *Storage) remove(ctx context.Context, provider, token, id string) error {
	e := s.config.Endpoints
	var err error
	switch provider {
	case GoogleDrive:
		_, _, err = s.do(ctx, provider, call{method: "DELETE", target: e.GoogleDrive + "/files/" + url.PathEscape(id), token: token})
	case Dropbox:
		body, _ := json.Marshal(map[string]string{"path": id})
		_, _, err = s.do(ctx, provider, call{method: "POST", target: e.DropboxAPI + "/files/delete_v2", token: token, contentType: "application/json", body: body})
		var p *providerError
		if errors.As(err, &p) && p.Status == 409 && p.Reason == "path_lookup" {
			return nil // not found
		}
	default:
		_, _, err = s.do(ctx, provider, call{method: "DELETE", target: e.Graph + "/me/drive/items/" + url.PathEscape(id), token: token})
	}
	var p *providerError
	if errors.As(err, &p) && (p.Status == 404 || p.Status == 410) {
		return nil
	}
	return err
}

// OneDrive: one request; Graph makes the folders on the way.
func (s *Storage) putOneDrive(ctx context.Context, token string, folders []string, name string, data []byte) (remoteFile, error) {
	parts := make([]string, 0, len(folders)+1)
	for _, f := range append(append([]string{}, folders...), name) {
		parts = append(parts, url.PathEscape(f))
	}
	target := s.config.Endpoints.Graph + "/me/drive/root:/" + strings.Join(parts, "/") + ":/content?@microsoft.graph.conflictBehavior=replace"
	raw, _, err := s.do(ctx, OneDrive, call{method: "PUT", target: target, token: token, contentType: "application/octet-stream", body: data})
	if err != nil {
		return remoteFile{}, err
	}
	var item struct{ ID, WebURL string }
	if json.Unmarshal(raw, &item) != nil || item.ID == "" {
		return remoteFile{}, errors.New(ProviderName(OneDrive) + " did not say where the receipt was put")
	}
	return remoteFile{ID: item.ID, URL: item.WebURL}, nil
}

// Dropbox: one request; it makes the folders on the way.
func (s *Storage) putDropbox(ctx context.Context, token string, folders []string, name string, data []byte) (remoteFile, error) {
	full := "/" + strings.Join(append(append([]string{}, folders...), name), "/")
	arg := asciiJSON(map[string]any{"path": full, "mode": "overwrite", "autorename": false, "mute": true})
	raw, _, err := s.do(ctx, Dropbox, call{method: "POST", target: s.config.Endpoints.DropboxContent + "/files/upload", token: token,
		contentType: "application/octet-stream", body: data, header: map[string]string{"Dropbox-API-Arg": arg}})
	if err != nil {
		return remoteFile{}, err
	}
	var meta struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &meta) != nil || meta.ID == "" {
		return remoteFile{}, errors.New(ProviderName(Dropbox) + " did not say where the receipt was put")
	}
	escaped := make([]string, 0, len(folders))
	for _, f := range folders {
		escaped = append(escaped, url.PathEscape(f))
	}
	return remoteFile{ID: meta.ID, URL: "https://www.dropbox.com/home/" + strings.Join(escaped, "/") + "?preview=" + url.QueryEscape(name)}, nil
}

// asciiJSON is JSON with every character past ASCII escaped: Dropbox takes
// its arguments in a header, which must be ASCII.
func asciiJSON(v any) string {
	raw, _ := json.Marshal(v)
	var b strings.Builder
	for _, r := range string(raw) {
		if r < 0x80 {
			b.WriteRune(r)
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&b, `\u%04x`, u)
		}
	}
	return b.String()
}

const googleFolder = "application/vnd.google-apps.folder"

// Google Drive: folders are found or made one by one (HyPlug sees only
// what it made: the drive.file permission), then the file is uploaded in a
// resumable session.
func (s *Storage) putGoogle(ctx context.Context, token string, folders []string, name, contentType string, data []byte) (remoteFile, error) {
	parent := "root"
	for _, f := range folders {
		id, err := s.googleFind(ctx, token, parent, f, true)
		if err == nil && id == "" {
			id, err = s.googleFolder(ctx, token, parent, f)
		}
		if err != nil {
			return remoteFile{}, err
		}
		parent = id
	}
	// A file of the same name (an earlier try whose answer was lost) is replaced.
	if old, err := s.googleFind(ctx, token, parent, name, false); err != nil {
		return remoteFile{}, err
	} else if old != "" {
		if err := s.remove(ctx, GoogleDrive, token, old); err != nil {
			return remoteFile{}, err
		}
	}
	e := s.config.Endpoints
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{parent}})
	_, header, err := s.do(ctx, GoogleDrive, call{method: "POST", target: e.GoogleUpload + "/files?uploadType=resumable&fields=id,webViewLink", token: token,
		contentType: "application/json; charset=UTF-8", body: meta,
		header: map[string]string{"X-Upload-Content-Type": contentType, "X-Upload-Content-Length": strconv.Itoa(len(data))}})
	if err != nil {
		return remoteFile{}, err
	}
	session := header.Get("Location")
	if !sameOrigin(session, e.GoogleUpload) {
		return remoteFile{}, errors.New(ProviderName(GoogleDrive) + " gave an upload address that is not its own")
	}
	raw, _, err := s.do(ctx, GoogleDrive, call{method: "PUT", target: session, token: token, contentType: contentType, body: data})
	if err != nil {
		return remoteFile{}, err
	}
	var file struct{ ID, WebViewLink string }
	if json.Unmarshal(raw, &file) != nil || file.ID == "" {
		return remoteFile{}, errors.New(ProviderName(GoogleDrive) + " did not say where the receipt was put")
	}
	return remoteFile{ID: file.ID, URL: file.WebViewLink}, nil
}

func (s *Storage) googleFind(ctx context.Context, token, parent, name string, folder bool) (string, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", driveQuoted(name), driveQuoted(parent))
	if folder {
		q += " and mimeType = '" + googleFolder + "'"
	} else {
		q += " and mimeType != '" + googleFolder + "'"
	}
	target := s.config.Endpoints.GoogleDrive + "/files?" + url.Values{"q": {q}, "fields": {"files(id)"}, "pageSize": {"1"}, "spaces": {"drive"}}.Encode()
	raw, _, err := s.do(ctx, GoogleDrive, call{method: "GET", target: target, token: token})
	if err != nil {
		return "", err
	}
	var list struct{ Files []struct{ ID string } }
	if json.Unmarshal(raw, &list) != nil {
		return "", errors.New(ProviderName(GoogleDrive) + "'s folder list could not be read")
	}
	if len(list.Files) == 0 {
		return "", nil
	}
	return list.Files[0].ID, nil
}

func (s *Storage) googleFolder(ctx context.Context, token, parent, name string) (string, error) {
	body, _ := json.Marshal(map[string]any{"name": name, "mimeType": googleFolder, "parents": []string{parent}})
	raw, _, err := s.do(ctx, GoogleDrive, call{method: "POST", target: s.config.Endpoints.GoogleDrive + "/files?fields=id", token: token, contentType: "application/json", body: body})
	if err != nil {
		return "", err
	}
	var f struct{ ID string }
	if json.Unmarshal(raw, &f) != nil || f.ID == "" {
		return "", errors.New(ProviderName(GoogleDrive) + " did not make the folder " + name)
	}
	return f.ID, nil
}

// driveQuoted escapes a value for a Drive query ('...').
func driveQuoted(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `'`, `\'`)
}

func sameOrigin(target, base string) bool {
	t, err1 := url.Parse(target)
	b, err2 := url.Parse(base)
	return err1 == nil && err2 == nil && t.Scheme == b.Scheme && t.Host == b.Host && b.Host != ""
}
