package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func beginOAuth(t *testing.T, a *App) string {
	t.Helper()
	w := call(a, "POST", "/api/oauth", `{"clientId":"  synthetic-client  ","clientSecret":"  synthetic-secret  "}`, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result struct{ URL string }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("client_id") != "synthetic-client" || u.Query().Get("redirect_uri") != a.origin+"/oauth/callback" || u.Query().Get("scope") != "read offline_access" {
		t.Fatal("authorization request differs from registered application settings")
	}
	if strings.Contains(result.URL, "synthetic-secret") {
		t.Fatal("secret placed in authorization URL")
	}
	return u.Query().Get("state")
}

func TestOAuthCompletesWithOptionalAccountNameMissing(t *testing.T) {
	a := testApp(t)
	a.clientHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == tokenEndpoint {
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Fatal("incorrect token request")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("client_id") != "synthetic-client" || r.PostForm.Get("client_secret") != "synthetic-secret" || r.PostForm.Get("redirect_uri") != a.origin+"/oauth/callback" {
				t.Fatal("token request lost application settings")
			}
			return response(200, `{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","api":"https://api.jobadder.com/v2/","account":9007199254740993,"instance":"synthetic-instance"}`), nil
		}
		if r.URL.Path != "/v2/users/current" || r.Header.Get("Authorization") != "Bearer synthetic-access" {
			t.Fatal("unverified identity request")
		}
		return response(200, `{"userId":7,"accountName":null,"firstName":"Synthetic"}`), nil
	})}
	state := beginOAuth(t, a)
	w := call(a, "GET", "/oauth/callback?"+url.Values{"state": {state}, "code": {"synthetic-code"}}.Encode(), "", false)
	if w.Code != 303 || w.Header().Get("Location") != a.origin+"/" {
		t.Fatal("callback did not remove code from URL")
	}
	c, err := a.store.Config()
	if err != nil || c.Fingerprint == "" || c.AccountID != "9007199254740993" || c.Instance != "synthetic-instance" || c.UserID != "7" || c.RefreshToken != "synthetic-refresh" || c.Account != "JobAdder account 9007199254740993" {
		t.Fatal("verified OAuth identity was not saved", err)
	}
	w = call(a, "GET", "/api/status", "", true)
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["connected"] != true || status["connectionError"] != "" {
		t.Fatal("successful connection remained disconnected")
	}
	for _, secret := range []string{"synthetic-access", "synthetic-refresh", "synthetic-secret", "synthetic-code"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret exposed in browser status")
		}
	}
	if call(a, "GET", "/oauth/callback?"+url.Values{"state": {state}, "code": {"synthetic-code"}}.Encode(), "", false).Code != 400 {
		t.Fatal("authorization code replay accepted")
	}
}

func TestOAuthShowsSafeFailureStageAndPreservesExistingConnection(t *testing.T) {
	for _, test := range []struct {
		name        string
		tokenStatus int
		tokenBody   string
		userStatus  int
		userBody    string
		message     string
	}{
		{"credentials", 400, `{"error":"invalid_client","error_description":"never-echo-private-data"}`, 200, `{}`, "Token exchange: JobAdder rejected the app credentials"},
		{"code", 400, `{"error":"invalid_grant","error_description":"never-echo-private-data"}`, 200, `{}`, "Token exchange: JobAdder rejected the authorization code"},
		{"permissions", 200, `{"access_token":"synthetic-access","token_type":"Bearer","api":"https://api.jobadder.com/v2","account":101}`, 403, `{"error":"never-echo-private-data"}`, "Account verification: JobAdder denied the current-user check (HTTP 403)"},
		{"identity", 200, `{"access_token":"synthetic-access","token_type":"Bearer","api":"https://api.jobadder.com/v2"}`, 200, `{"userId":7,"accountName":null}`, "Account verification: JobAdder omitted the account name"},
		{"unknown", 400, `{"error":"never-echo-private-data","error_description":"never-echo-private-data"}`, 200, `{}`, "Token exchange: JobAdder rejected the token exchange"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := testApp(t)
			old := Connection{API: "https://api.jobadder.com/v2", AccessToken: "existing-private-token", Account: "Previous synthetic agency", Fingerprint: "previous-source"}
			if err := a.store.SaveConfig(old); err != nil {
				t.Fatal(err)
			}
			a.connection = old
			a.clientHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == tokenEndpoint {
					return response(test.tokenStatus, test.tokenBody), nil
				}
				return response(test.userStatus, test.userBody), nil
			})}
			state := beginOAuth(t, a)
			w := call(a, "GET", "/oauth/callback?"+url.Values{"state": {state}, "code": {"synthetic-code"}}.Encode(), "", false)
			if w.Code != 303 {
				t.Fatal("failed callback did not return to tool")
			}
			w = call(a, "GET", "/api/status", "", true)
			if !strings.Contains(w.Body.String(), test.message) {
				t.Fatal("failure stage hidden", w.Body.String())
			}
			for _, private := range []string{"never-echo-private-data", "existing-private-token", "synthetic-access", "synthetic-secret", "synthetic-code"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("provider data or credentials leaked")
				}
			}
			stored, err := a.store.Config()
			if err != nil || stored != old || a.connection != old || a.connecting {
				t.Fatal("failed login replaced or blocked previous connection", err)
			}
			beginOAuth(t, a)
			if a.connectionError != "" {
				t.Fatal("new sign-in retained stale error")
			}
		})
	}
}

func TestOAuthDenialNeverEchoesProviderDescription(t *testing.T) {
	a := testApp(t)
	state := beginOAuth(t, a)
	w := call(a, "GET", "/oauth/callback?"+url.Values{"state": {state}, "error": {"access_denied"}, "error_description": {"private-provider-data"}}.Encode(), "", false)
	if w.Code != 303 || !strings.Contains(a.connectionError, "Grant API Access") || strings.Contains(a.connectionError, "private-provider-data") {
		t.Fatal("unsafe or missing denial guidance")
	}
}

func TestOAuthCallbackCannotRaceActiveTransfer(t *testing.T) {
	a := testApp(t)
	state := beginOAuth(t, a)
	a.transferring = true
	if w := call(a, "GET", "/oauth/callback?"+url.Values{"state": {state}, "code": {"synthetic-code"}}.Encode(), "", false); w.Code != 400 {
		t.Fatal("connection changed during export/intake")
	}
	if a.oauthState != state {
		t.Fatal("rejected callback consumed active authorization")
	}
}

func TestRefreshedCredentialsStayBoundToSource(t *testing.T) {
	for _, body := range []string{
		`{"access_token":"next","refresh_token":"rotated","token_type":"Bearer","api":"https://api.jobadder.com/v2","account":102,"instance":"region"}`,
		`{"access_token":"next","refresh_token":"rotated","token_type":"Bearer","api":"https://api.jobadder.com/v2","account":101,"instance":"different"}`,
		`{"access_token":"next","refresh_token":"rotated","token_type":"Bearer","api":"https://api.au1.jobadder.com/v2","account":101,"instance":"region"}`,
	} {
		saved := false
		c := &Client{Config: Connection{API: "https://api.jobadder.com/v2", AccessToken: "old", RefreshToken: "refresh", ClientID: "id", ClientSecret: "secret", AccountID: "101", Instance: "region", UserID: "7"}, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return response(200, body), nil })}, Save: func(Connection) error { saved = true; return nil }}
		if err := c.refresh(context.Background()); err == nil || saved || c.Config.AccessToken != "old" {
			t.Fatal("refresh switched source identity")
		}
	}
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2", RefreshToken: "refresh", ClientID: "id", ClientSecret: "secret", AccountID: "101", Instance: "region", UserID: "7"}, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, `{"access_token":"next","token_type":"Bearer","api":"https://api.jobadder.com/v2"}`), nil
	})}}
	if err := c.refresh(context.Background()); err != nil || c.Config.AccountID != "101" || c.Config.Instance != "region" || c.Config.UserID != "7" {
		t.Fatal("refresh lost verified identity metadata", err)
	}
}

func TestExpiredOAuthStillFailsClosed(t *testing.T) {
	a := testApp(t)
	state := beginOAuth(t, a)
	a.oauthUntil = time.Now().Add(-time.Second)
	if w := call(a, "GET", "/oauth/callback?state="+state+"&code=synthetic", "", false); w.Code != 400 || a.connection.Fingerprint != "" {
		t.Fatal("expired authorization accepted")
	}
}
