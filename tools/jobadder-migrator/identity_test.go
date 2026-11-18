package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func identityClient(c Connection, body string) *Client {
	return &Client{Config: c, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return response(200, body), nil })}}
}

func TestIdentityRequiresVerifiedAgencyNamespace(t *testing.T) {
	for _, c := range []Connection{
		{API: "https://api.jobadder.com/v2"},
		{API: "https://api.jobadder.com/v2", AccountID: "101"},
		{API: "https://api.jobadder.com/v2", Instance: "region"},
	} {
		if _, err := identify(context.Background(), identityClient(c, `{"userId":7,"accountName":null}`)); err == nil {
			t.Fatal("user ID alone or partial token metadata accepted as source account")
		}
	}
	for _, body := range []string{`{"accountName":"Sample"}`, `{"userId":0,"accountName":"Sample"}`, `{"userId":-1,"accountName":"Sample"}`, `{"userId":1.5,"accountName":"Sample"}`} {
		if _, err := identify(context.Background(), identityClient(Connection{API: "https://api.jobadder.com/v2", AccountID: "101", Instance: "region"}, body)); err == nil {
			t.Fatal("invalid current-user identity accepted")
		}
	}
}

func TestStableOAuthSourceSurvivesMissingOrRenamedAccountName(t *testing.T) {
	config := Connection{API: "https://api.jobadder.com/v2", AccountID: "101", Instance: "region"}
	a, err := identify(context.Background(), identityClient(config, `{"userId":7,"accountName":"Synthetic agency"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := identify(context.Background(), identityClient(config, `{"userId":7,"accountName":null}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := identify(context.Background(), identityClient(config, `{"userId":7,"accountName":"Renamed synthetic agency"}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint || b.Fingerprint != c.Fingerprint {
		t.Fatal("display name is still used as OAuth source identity")
	}
	for _, changed := range []Connection{
		{API: config.API, AccountID: "102", Instance: "region"},
		{API: config.API, AccountID: "101", Instance: "different"},
		{API: "https://api.au1.jobadder.com/v2", AccountID: "101", Instance: "region"},
	} {
		next, err := identify(context.Background(), identityClient(changed, `{"userId":7,"accountName":"Synthetic agency"}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := preserveSourceKey(a, &next); err == nil {
			t.Fatal("source namespace changed without separate workspace")
		}
	}
	next, err := identify(context.Background(), identityClient(config, `{"userId":8,"accountName":"Synthetic agency"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := preserveSourceKey(a, &next); err == nil {
		t.Fatal("source API user changed")
	}
}

func TestLegacyWorkspaceKeepsItsCheckpointKeyOnOAuthUpgrade(t *testing.T) {
	a := testApp(t)
	legacy := sourceHash("https://api.jobadder.com/v2\x007\x00Synthetic agency")
	old := Connection{API: "https://api.jobadder.com/v2", Account: "Synthetic agency", Fingerprint: legacy}
	if err := a.store.SaveConfig(old); err != nil {
		t.Fatal(err)
	}
	a.clientHTTP = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, `{"userId":7,"accountName":"Synthetic agency"}`), nil
	})}
	if err := a.accept(context.Background(), Connection{API: old.API, AccountID: "101", Instance: "region"}); err != nil {
		t.Fatal(err)
	}
	stored, err := a.store.Config()
	if err != nil || stored.Fingerprint != legacy || stored.AccountID != "101" || stored.Instance != "region" || stored.UserID != "7" {
		t.Fatal("existing source key or verified identity lost", err)
	}
	a.clientHTTP = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, `{"userId":7,"accountName":null}`), nil
	})}
	if err := a.accept(context.Background(), Connection{API: old.API, AccountID: "101", Instance: "region"}); err != nil {
		t.Fatal("nullable name broke upgraded workspace", err)
	}
	stored, _ = a.store.Config()
	if stored.Fingerprint != legacy {
		t.Fatal("saved extraction checkpoint namespace changed")
	}
	if err := a.accept(context.Background(), Connection{API: old.API, AccountID: "102", Instance: "region"}); err == nil {
		t.Fatal("upgraded source switched agencies")
	}
}

func TestTokenIdentityMetadataIsParsedExactlyAndValidated(t *testing.T) {
	for _, raw := range []string{`9007199254740993`, `"9007199254740993"`} {
		id, err := tokenAccountID([]byte(raw))
		if err != nil || id != "9007199254740993" {
			t.Fatal("account ID rounded", err)
		}
	}
	for _, raw := range []string{`-1`, `0`, `1.5`, `"private-data"`, `{}`, `[]`, `true`} {
		if _, err := tokenAccountID([]byte(raw)); err == nil || strings.Contains(err.Error(), "private-data") {
			t.Fatal("invalid or sensitive account metadata accepted/echoed")
		}
	}
	for _, raw := range []string{`"region-1"`, `17`, `"17"`} {
		id, err := tokenInstance([]byte(raw))
		if err != nil || id == "" {
			t.Fatal("valid instance metadata rejected", err)
		}
	}
	for _, raw := range []string{`true`, `{}`, `[]`, `""`, `"private-data\n"`} {
		if _, err := tokenInstance([]byte(raw)); err == nil || strings.Contains(err.Error(), "private-data") {
			t.Fatal("invalid or sensitive instance metadata accepted/echoed")
		}
	}
}
