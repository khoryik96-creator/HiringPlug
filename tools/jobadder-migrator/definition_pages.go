package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
)

// Categories supports Offset/Limit but has no totalCount. Preserve every page
// separately and publish a total only when the documented paging is exhausted.
func discoverDefinitionPages(ctx context.Context, s *Store, c *Client, run int64, r Resource) error {
	const limit = 1000
	seen := map[[32]byte]bool{}
	offset := 0
	for page := 0; page < 100; page++ {
		raw, code, requestErr := c.get(ctx, r.Path+"?Limit=1000&Offset="+strconv.Itoa(offset))
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if requestErr != nil {
			status := "failed"
			switch code {
			case 401:
				status = "reconnect"
			case 403:
				status = "denied"
			case 404:
				status = "unavailable"
			case 429:
				status = "rate_limited"
			}
			if err := s.SaveResult(run, r, status, nil, code, nil); err != nil {
				return errors.New("checkpoint could not be saved; inventory stopped")
			}
			var cooldown *CooldownError
			if errors.As(requestErr, &cooldown) {
				return cooldown
			}
			if code == 401 {
				return errors.New("connection expired; reconnect before resuming")
			}
			if code == 0 || code == 429 || code >= 500 {
				return errors.New("JobAdder is unavailable; resume this inventory later")
			}
			return nil
		}
		var envelope struct {
			Items json.RawMessage `json:"items"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		var items []json.RawMessage
		valid := json.Unmarshal(raw, &envelope) == nil && envelope.Items != nil && json.Unmarshal(envelope.Items, &items) == nil
		digest := sha256.Sum256(envelope.Items)
		if !valid || seen[digest] || (len(items) == 0 && envelope.Links.Next != "") || len(items) > limit {
			if err := s.SaveResult(run, r, "needs_review", nil, code, raw); err != nil {
				return errors.New("checkpoint could not be saved; inventory stopped")
			}
			return errors.New("Category pagination could not be verified; original responses were retained for review")
		}
		seen[digest] = true
		offset += len(items)
		status := "definition_page"
		var count *int64
		if len(items) < limit && envelope.Links.Next == "" {
			n := int64(offset)
			count = &n
			status = "definitions"
		}
		if err := s.SaveResult(run, r, status, count, code, raw); err != nil {
			return errors.New("checkpoint could not be saved; inventory stopped")
		}
		if status == "definitions" {
			return nil
		}
	}
	return errors.New("Category pagination reached the discovery limit; original pages were retained for review")
}
