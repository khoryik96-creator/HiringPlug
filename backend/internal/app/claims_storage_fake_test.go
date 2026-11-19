package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims"
)

// fakeDrives plays Google Drive, OneDrive and Dropbox: their sign-in, the
// account, and files kept by path (folder names joined with "/").
type fakeDrives struct {
	mu        sync.Mutex
	url       string
	files     map[string]fakeFile // path -> file
	folders   map[string]string   // Google: folder id -> path
	nextID    int
	refreshes int
	expiresIn int    // the access tokens' lifetime (3600 when 0)
	fail      int    // the next uploads answer this status (0: none)
	failTimes int    // how many uploads fail (with fail)
	noRefresh bool   // the sign-in gives no refresh token
	refused   bool   // refreshing answers invalid_grant
	access    string // the access token handed out last
}

type fakeFile struct {
	ID   string
	Data []byte
}

type fakeStorageApps struct{}

func (fakeStorageApps) App(_ context.Context, _ pgx.Tx, _, provider string) (claims.App, error) {
	return claims.App{ClientID: "synthetic-" + provider, ClientSecret: "synthetic-secret"}, nil
}

func setupClaimStorage(t *testing.T, a *testApp) *fakeDrives {
	t.Helper()
	f := &fakeDrives{files: map[string]fakeFile{}, folders: map[string]string{"root": ""}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.url = server.URL
	e := server.URL
	a.modules.Claims.UseStorage(fakeStorageApps{}, claims.StorageConfig{
		KeysSecret: []byte("synthetic-claim-storage-key-0123456789abcdef"), PublicURL: "http://app.test", HTTP: server.Client(),
		Dropbox: claims.App{ClientID: "synthetic-dropbox", ClientSecret: "synthetic-secret"},
		Endpoints: claims.Endpoints{
			MicrosoftAuthorize: e + "/ms/authorize", MicrosoftToken: e + "/token", Graph: e + "/graph",
			GoogleAuthorize: e + "/google/authorize", GoogleToken: e + "/token", GoogleUserInfo: e + "/google/userinfo",
			GoogleDrive: e + "/drive", GoogleUpload: e + "/upload",
			DropboxAuthorize: e + "/dropbox/authorize", DropboxToken: e + "/token", DropboxAPI: e + "/dropbox/api", DropboxContent: e + "/dropbox/content",
		},
	})
	return f
}

// paths are the files kept, sorted.
func (f *fakeDrives) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.files))
	for p := range f.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (f *fakeDrives) file(path string) fakeFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[path]
}

func (f *fakeDrives) id() string {
	f.nextID++
	return "item-" + strconv.Itoa(f.nextID)
}

func (f *fakeDrives) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/token" || r.Header.Get("Authorization") == "Bearer "+f.access {
		return true
	}
	w.WriteHeader(401)
	_, _ = w.Write([]byte(`{"error":{"code":"InvalidAuthenticationToken"}}`))
	return false
}

// failing answers an upload with the planned failure, if any.
func (f *fakeDrives) failing(w http.ResponseWriter) bool {
	if f.failTimes == 0 {
		return false
	}
	f.failTimes--
	w.WriteHeader(f.fail)
	return true
}

var graphContent = regexp.MustCompile(`^/graph/me/drive/root:/(.+):/content$`)

func (f *fakeDrives) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if !f.authorized(w, r) {
		return
	}
	body, _ := io.ReadAll(r.Body)
	path := r.URL.EscapedPath()
	switch {
	case path == "/token":
		f.token(w, r, body)
	case path == "/graph/me":
		_, _ = w.Write([]byte(`{"mail":"Finance@Agency.test","userPrincipalName":"finance@agency.test"}`))
	case path == "/graph/me/drive":
		_, _ = w.Write([]byte(`{"id":"drive-1"}`))
	case path == "/google/userinfo":
		_, _ = w.Write([]byte(`{"email":"finance@agency.test"}`))
	case path == "/dropbox/api/users/get_current_account":
		_, _ = w.Write([]byte(`{"email":"finance@agency.test","name":{"display_name":"Finance"}}`))
	case graphContent.MatchString(path) && r.Method == "PUT":
		if f.failing(w) {
			return
		}
		name, _ := url.PathUnescape(graphContent.FindStringSubmatch(path)[1])
		item := fakeFile{ID: f.id(), Data: body}
		f.files[name] = item
		_ = json.NewEncoder(w).Encode(map[string]string{"id": item.ID, "webUrl": "https://onedrive.test/" + item.ID})
	case strings.HasPrefix(path, "/graph/me/drive/items/") && r.Method == "DELETE":
		f.deleteID(w, strings.TrimPrefix(path, "/graph/me/drive/items/"))
	case path == "/dropbox/content/files/upload":
		if f.failing(w) {
			return
		}
		var arg struct{ Path, Mode string }
		_ = json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg)
		item := fakeFile{ID: "id:" + f.id(), Data: body}
		f.files[strings.TrimPrefix(arg.Path, "/")] = item
		_ = json.NewEncoder(w).Encode(map[string]string{"id": item.ID, "path_display": arg.Path})
	case path == "/dropbox/api/files/delete_v2":
		var arg struct{ Path string }
		_ = json.Unmarshal(body, &arg)
		if !f.dropID(arg.Path) {
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"error_summary":"path_lookup/not_found/"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	default:
		f.google(w, r, path, body)
	}
}

func (f *fakeDrives) token(w http.ResponseWriter, r *http.Request, body []byte) {
	form, _ := url.ParseQuery(string(body))
	refresh := "synthetic-refresh-" + strconv.Itoa(f.refreshes)
	if form.Get("grant_type") == "refresh_token" {
		if f.refused {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		f.refreshes++
		refresh = "" // the refresh token is kept
	} else if f.noRefresh {
		refresh = ""
	}
	f.access = "synthetic-access-" + strconv.Itoa(f.refreshes)
	expires := f.expiresIn
	if expires == 0 {
		expires = 3600
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": f.access, "refresh_token": refresh, "token_type": "bearer", "expires_in": expires})
}

func (f *fakeDrives) deleteID(w http.ResponseWriter, id string) {
	if !f.dropID(id) {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func (f *fakeDrives) dropID(id string) bool {
	for p, file := range f.files {
		if file.ID == id {
			delete(f.files, p)
			return true
		}
	}
	return false
}

var driveQuery = regexp.MustCompile(`^name = '((?:[^'\\]|\\.)*)' and '([^']*)' in parents`)

// google plays Drive: folders by id, the file list query, resumable uploads.
func (f *fakeDrives) google(w http.ResponseWriter, r *http.Request, path string, body []byte) {
	switch {
	case path == "/drive/files" && r.Method == "GET":
		m := driveQuery.FindStringSubmatch(r.URL.Query().Get("q"))
		if m == nil {
			w.WriteHeader(400)
			return
		}
		name := strings.NewReplacer(`\'`, `'`, `\\`, `\`).Replace(m[1])
		full := strings.TrimPrefix(f.folders[m[2]]+"/"+name, "/")
		found := []map[string]string{}
		folder := strings.Contains(r.URL.Query().Get("q"), "mimeType = 'application/vnd.google-apps.folder'")
		for id, p := range f.folders {
			if folder && p == full && id != "root" {
				found = append(found, map[string]string{"id": id})
			}
		}
		if file, ok := f.files[full]; ok && !folder {
			found = append(found, map[string]string{"id": file.ID})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": found})
	case path == "/drive/files" && r.Method == "POST":
		var meta struct {
			Name    string
			Parents []string
		}
		_ = json.Unmarshal(body, &meta)
		id := f.id()
		f.folders[id] = strings.TrimPrefix(f.folders[meta.Parents[0]]+"/"+meta.Name, "/")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	case strings.HasPrefix(path, "/drive/files/") && r.Method == "DELETE":
		f.deleteID(w, strings.TrimPrefix(path, "/drive/files/"))
	case path == "/upload/files" && r.URL.Query().Get("uploadType") == "resumable":
		var meta struct {
			Name    string
			Parents []string
		}
		_ = json.Unmarshal(body, &meta)
		session := url.QueryEscape(strings.TrimPrefix(f.folders[meta.Parents[0]]+"/"+meta.Name, "/"))
		w.Header().Set("Location", f.url+"/upload/session?to="+session)
	case path == "/upload/session" && r.Method == "PUT":
		if f.failing(w) {
			return
		}
		item := fakeFile{ID: f.id(), Data: body}
		f.files[r.URL.Query().Get("to")] = item
		_ = json.NewEncoder(w).Encode(map[string]string{"id": item.ID, "webViewLink": "https://drive.test/" + item.ID})
	default:
		w.WriteHeader(404)
		_, _ = fmt.Fprintf(w, `{"error":"no such fake route %s %s"}`, r.Method, path)
	}
}
