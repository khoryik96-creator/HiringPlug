package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

func identify(ctx context.Context, c *Client) (Connection, error) {
	raw, _, err := c.get(ctx, "/users/current")
	if err != nil {
		return Connection{}, errors.New("could not verify the JobAdder account; check the connection and user-read permission")
	}
	var identity struct {
		ID      json.Number `json:"userId"`
		Account string      `json:"accountName"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.ID == "" || identity.Account == "" {
		return Connection{}, errors.New("JobAdder returned no account identity; connection was not saved")
	}
	// Preserve exact source responses elsewhere; these values only bind the workspace.
	sum := sha256.Sum256([]byte(c.Config.API + "\x00" + identity.ID.String() + "\x00" + identity.Account))
	out := c.Config
	out.Account = identity.Account
	out.Fingerprint = hex.EncodeToString(sum[:])
	return out, nil
}
func countResponse(raw []byte, kind string) (int64, error) {
	if kind == "definitions" && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			return int64(len(list)), nil
		}
		return 0, errors.New("invalid definition response")
	}

	var envelope struct {
		Count *json.Number    `json:"totalCount"`
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return 0, errors.New("invalid collection response")
	}
	if envelope.Count != nil {
		n, err := strconv.ParseInt(envelope.Count.String(), 10, 64)
		if err != nil || n < 0 {
			return 0, errors.New("invalid reported count")
		}
		return n, nil
	}
	// Never infer a collection total from a page length, or mistake missing or
	// inaccessible data for zero.
	return 0, errors.New("no authoritative total count returned")
}
func discover(ctx context.Context, s *Store, c *Client, run int64, resources []Resource, resume bool, progress func(string)) error {
	done := map[string]bool{}
	if resume {
		r, err := s.Report()
		if err != nil {
			return err
		}
		for _, v := range r.Results {
			done[v.Key] = v.Status == "counted" || v.Status == "definitions"
		}
	}
	for _, r := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if done[r.Key] {
			continue
		}
		progress(r.Label)
		path := r.Path
		if r.Kind == "count" {
			path += "?Limit=0"
		}
		raw, httpCode, err := c.get(ctx, path)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		status := "failed"
		var n *int64
		if err != nil {
			switch httpCode {
			case 401:
				status = "reconnect"
			case 403:
				status = "denied"
			case 404:
				status = "unavailable"
			case 429:
				status = "rate_limited"
			}
		} else {
			value, countErr := countResponse(raw, r.Kind)
			if countErr == nil {
				n = &value
				status = "counted"
				if r.Kind == "definitions" {
					status = "definitions"
				}
			} else {
				status = "needs_review"
			}
		}
		// raw is the original response bytes, not a decoded/re-encoded object.
		if saveErr := s.SaveResult(run, r, status, n, httpCode, raw); saveErr != nil {
			return errors.New("checkpoint could not be saved; inventory stopped")
		}
		if httpCode == 401 {
			return errors.New("connection expired; reconnect before resuming")
		}
		if httpCode == 429 || httpCode >= 500 || (err != nil && httpCode == 0) {
			return errors.New("JobAdder is unavailable; resume this inventory later")
		}
	}
	progress("")
	return nil
}
