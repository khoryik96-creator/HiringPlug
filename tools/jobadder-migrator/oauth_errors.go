package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Provider descriptions may contain credentials or personal data. Classify only
// known OAuth error codes; never echo a raw response, URL or description.
func authorizationFailure(status int, body io.Reader) error {
	raw, _ := readLimited(body, 128<<10)
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &payload)
	message := "JobAdder rejected the token exchange; check the app settings and try a fresh sign-in"
	switch payload.Error {
	case "invalid_client":
		message = "JobAdder rejected the app credentials. Recopy the Client ID and current Client Secret from the same JobAdder application"
	case "invalid_grant":
		message = "JobAdder rejected the authorization code. Confirm the exact registered callback, then start a fresh sign-in; codes expire and can be used only once"
	case "invalid_scope":
		message = "JobAdder did not allow the requested read/offline_access scopes. Check the application's allowed scopes"
	case "unauthorized_client":
		message = "JobAdder did not authorize this application for authorization-code access. Check its application approval and grant settings"
	case "access_denied":
		message = "JobAdder denied API access. Sign in with an agency admin or a user with Grant API Access permission"
	}
	return fmt.Errorf("%s (HTTP %d).", message, status)
}
