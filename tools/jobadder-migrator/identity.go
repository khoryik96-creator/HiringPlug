package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func tokenAccountID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var value json.Number
	if json.Unmarshal(raw, &value) != nil {
		return "", errors.New("JobAdder returned an invalid account identifier")
	}
	n, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || n <= 0 {
		return "", errors.New("JobAdder returned an invalid account identifier")
	}
	return strconv.FormatInt(n, 10), nil
}
func tokenInstance(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		var number json.Number
		if json.Unmarshal(raw, &number) != nil {
			return "", errors.New("JobAdder returned an invalid instance identifier")
		}
		value = number.String()
	}
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("JobAdder returned an invalid instance identifier")
	}
	return value, nil
}
func sourceHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func identify(ctx context.Context, c *Client) (Connection, error) {
	raw, status, err := c.get(ctx, "/users/current")
	if err != nil {
		var cooldown *CooldownError
		if errors.As(err, &cooldown) {
			return Connection{}, err
		}
		if status == 403 {
			return Connection{}, errors.New("JobAdder denied the current-user check (HTTP 403). Ask your JobAdder admin to allow user-read/API access, then reconnect")
		}
		if status == 401 {
			return Connection{}, errors.New("JobAdder rejected the token during the current-user check (HTTP 401). Start sign-in again")
		}
		return Connection{}, fmt.Errorf("could not verify the JobAdder account (HTTP %d); check your connection and user-read permission", status)
	}
	var identity struct {
		ID      json.Number `json:"userId"`
		Account string      `json:"accountName"`
	}
	if json.Unmarshal(raw, &identity) != nil {
		return Connection{}, errors.New("JobAdder returned an invalid current-user response; connection was not saved")
	}
	user, err := strconv.ParseInt(identity.ID.String(), 10, 64)
	if err != nil || user <= 0 {
		return Connection{}, errors.New("JobAdder returned no valid current-user identifier; connection was not saved")
	}
	out := c.Config
	out.UserID = strconv.FormatInt(user, 10)
	out.Account = identity.Account
	out.legacyFingerprint = ""
	if identity.Account != "" {
		out.legacyFingerprint = sourceHash(out.API + "\x00" + identity.ID.String() + "\x00" + identity.Account)
	}
	if out.AccountID != "" && out.Instance != "" {
		out.Fingerprint = sourceHash("jobadder-source-v2\x00" + out.API + "\x00" + out.Instance + "\x00" + out.AccountID + "\x00" + out.UserID)
		if out.Account == "" {
			out.Account = "JobAdder account " + out.AccountID
		}
	} else {
		if identity.Account == "" {
			return Connection{}, errors.New("JobAdder omitted the account name and no complete verified OAuth account/instance identity is available. Connect using Sign in to JobAdder; a user ID alone cannot safely identify the source agency")
		}
		out.Fingerprint = out.legacyFingerprint
	}
	return out, nil
}

// Retain existing source keys and checkpoints while binding new OAuth grants to
// provider-issued account/instance/user IDs. Never use unverified token claims.
func preserveSourceKey(previous Connection, next *Connection) error {
	if previous.Fingerprint == "" {
		return nil
	}
	if previous.AccountID != "" && (previous.AccountID != next.AccountID || (previous.Instance != "" && previous.Instance != next.Instance)) {
		return errors.New("this workspace belongs to another JobAdder account; use a separate workspace")
	}
	if previous.Fingerprint == next.Fingerprint {
		return nil
	}
	verified := previous.AccountID != "" && previous.Instance != "" && previous.AccountID == next.AccountID && previous.API == next.API && previous.Instance == next.Instance && previous.UserID != "" && previous.UserID == next.UserID
	legacy := previous.Fingerprint == next.legacyFingerprint && next.legacyFingerprint != ""
	if !verified && !legacy {
		return errors.New("this workspace belongs to another account or API user; choose a separate workspace")
	}
	next.Fingerprint = previous.Fingerprint
	return nil
}
