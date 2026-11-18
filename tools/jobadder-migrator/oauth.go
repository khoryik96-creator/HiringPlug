package main

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *App) authorize(w http.ResponseWriter, r *http.Request) {
	if !a.available() {
		failure(w, "Unlock the workspace and pause any inventory first")
		return
	}
	defer a.connectionDone()
	var in struct {
		ID     string `json:"clientId"`
		Secret string `json:"clientSecret"`
	}
	if !input(w, r, &in) {
		return
	}
	in.ID = strings.TrimSpace(in.ID)
	in.Secret = strings.TrimSpace(in.Secret)
	if in.ID == "" || in.Secret == "" {
		failure(w, "Enter the JobAdder app client ID and secret")
		return
	}
	a.mu.Lock()
	state := randomID()
	a.oauthState = state
	a.oauthUntil = time.Now().Add(10 * time.Minute)
	a.pending = Connection{ClientID: in.ID, ClientSecret: in.Secret}
	a.connectionError = ""
	a.mu.Unlock()
	q := url.Values{"response_type": {"code"}, "client_id": {in.ID}, "scope": {"read offline_access"}, "redirect_uri": {a.origin + "/oauth/callback"}, "state": {state}}
	reply(w, 200, map[string]string{"url": "https://id.jobadder.com/connect/authorize?" + q.Encode()})
}
func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	state := a.oauthState
	valid := state != "" && time.Now().Before(a.oauthUntil) && len(r.URL.Query()["state"]) == 1 && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) == 1 && a.unlocked && !a.running && !a.connecting && !a.transferring && !a.closing
	c := a.pending
	if valid {
		a.connecting = true
		a.oauthState = ""
		a.pending = Connection{}
	}
	a.mu.Unlock()
	if !valid {
		http.Error(w, "Connection request expired or invalid. Start again from the migration tool.", 400)
		return
	}
	defer a.connectionDone()
	message := "JobAdder did not return an authorization code. Start sign-in again from this tool."
	switch r.URL.Query().Get("error") {
	case "invalid_scope":
		message = "JobAdder did not allow the requested read/offline_access scopes. Check the application settings, then start sign-in again."
	case "unauthorized_client":
		message = "JobAdder did not authorize this application. Check its approval and authorization-code grant settings."
	case "access_denied":
		message = "JobAdder access was not granted. Sign in with your agency's JobAdder admin or a user with Grant API Access permission, then approve the connection."
	}
	if len(r.URL.Query()["code"]) == 1 && r.URL.Query().Get("code") != "" && r.URL.Query().Get("error") == "" {
		next, err := tokenExchange(r.Context(), a.clientHTTP, tokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "code": {r.URL.Query().Get("code")}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "redirect_uri": {a.origin + "/oauth/callback"}})
		if err != nil {
			message = "Token exchange: " + err.Error()
		} else {
			next.ClientID = c.ClientID
			next.ClientSecret = c.ClientSecret
			if err = a.accept(r.Context(), next); err != nil {
				message = "Account verification: " + err.Error()
			} else {
				message = ""
			}
		}
	}
	if message != "" {
		a.mu.Lock()
		if a.connection.Fingerprint != "" {
			message += " Your existing JobAdder connection was kept."
		}
		a.connectionError = message
		a.mu.Unlock()
	}
	http.Redirect(w, r, a.origin+"/", http.StatusSeeOther)
}
