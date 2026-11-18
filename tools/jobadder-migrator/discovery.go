package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

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
		Links struct {
			First string `json:"first"`
			Next  string `json:"next"`
			Prev  string `json:"prev"`
			Last  string `json:"last"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return 0, errors.New("invalid collection response")
	}
	if kind == "definitions" {
		// The catalogue's documented definition endpoints are unpaged items
		// envelopes. Unexpected pagination must not be called a complete list.
		if envelope.Links.First != "" || envelope.Links.Next != "" || envelope.Links.Prev != "" || envelope.Links.Last != "" {
			return 0, errors.New("definition response requires pagination review")
		}
		if envelope.Items != nil {
			var items []json.RawMessage
			if err := json.Unmarshal(envelope.Items, &items); err != nil {
				return 0, errors.New("invalid definition items")
			}
			if envelope.Count != nil {
				total, err := strconv.ParseInt(envelope.Count.String(), 10, 64)
				if err != nil || total != int64(len(items)) {
					return 0, errors.New("definition response is incomplete")
				}
			}
			return int64(len(items)), nil
		}
		return 0, errors.New("definition response has no items")
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
	done := map[string]Result{}
	if resume {
		r, err := s.Report()
		if err != nil {
			return err
		}
		for _, v := range r.Results {
			done[v.Key] = v
		}
	}
	for _, r := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		previous := done[r.Key]
		if previous.Kind == r.Kind && (previous.Status == "counted" || previous.Status == "definitions" || previous.Status == "deferred") {
			continue
		}
		progress(r.Label)
		if r.Kind == "paged_definitions" {
			if err := discoverDefinitionPages(ctx, s, c, run, r); err != nil {
				return err
			}
			continue
		}
		if r.Kind == "deferred" {
			if err := s.SaveResult(run, r, "deferred", nil, 0, nil); err != nil {
				return errors.New("checkpoint could not be saved; inventory stopped")
			}
			continue
		}
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
			var cooldown *CooldownError
			if errors.As(err, &cooldown) {
				status = "rate_limited"
			}
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
		var cooldown *CooldownError
		if errors.As(err, &cooldown) {
			return cooldown
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
