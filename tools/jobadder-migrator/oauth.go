package main

import (
	"crypto/subtle"
	"net/http"
	"net/url"
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
	if in.ID == "" || in.Secret == "" {
		failure(w, "Enter the JobAdder app client ID and secret")
		return
	}
	a.mu.Lock()
	state := randomID()
	a.oauthState = state
	a.oauthUntil = time.Now().Add(10 * time.Minute)
	a.pending = Connection{ClientID: in.ID, ClientSecret: in.Secret}
	a.mu.Unlock()
	q := url.Values{"response_type": {"code"}, "client_id": {in.ID}, "scope": {"read offline_access"}, "redirect_uri": {a.origin + "/oauth/callback"}, "state": {state}}
	reply(w, 200, map[string]string{"url": "https://id.jobadder.com/connect/authorize?" + q.Encode()})
}
func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	state := a.oauthState
	valid := state != "" && time.Now().Before(a.oauthUntil) && len(r.URL.Query()["state"]) == 1 && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) == 1 && !a.running && !a.connecting
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
	success := false
	if len(r.URL.Query()["code"]) == 1 && r.URL.Query().Get("code") != "" && r.URL.Query().Get("error") == "" {
		next, err := tokenExchange(r.Context(), a.clientHTTP, tokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "code": {r.URL.Query().Get("code")}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "redirect_uri": {a.origin + "/oauth/callback"}})
		if err == nil {
			next.ClientID = c.ClientID
			next.ClientSecret = c.ClientSecret
			success = a.accept(r.Context(), next) == nil
		}
	}
	if !success {
		a.mu.Lock()
		a.message = "JobAdder sign-in could not complete. Check the registered callback and read permissions, then connect again. Your existing connection was kept."
		a.mu.Unlock()
	}
	http.Redirect(w, r, a.origin+"/", http.StatusSeeOther)
}
