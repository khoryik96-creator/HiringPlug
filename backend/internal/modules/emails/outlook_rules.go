package emails

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

type outlookRuleActions struct {
	MoveToFolder        string `json:"moveToFolder,omitempty"`
	StopProcessingRules bool   `json:"stopProcessingRules,omitempty"`
}

type outlookRule struct {
	ID          string             `json:"id,omitempty"`
	DisplayName string             `json:"displayName"`
	Sequence    int32              `json:"sequence"`
	IsEnabled   bool               `json:"isEnabled"`
	IsReadOnly  bool               `json:"isReadOnly,omitempty"`
	HasError    bool               `json:"hasError,omitempty"`
	Actions     outlookRuleActions `json:"actions"`
}

type outlookRuleAddress struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type outlookRuleConditions struct {
	FromAddresses   []outlookRuleAddress `json:"fromAddresses,omitempty"`
	SubjectContains []string             `json:"subjectContains,omitempty"`
}

func (s *Service) outlookRulesPath() string {
	return s.outlookRoot() + "/mailFolders/inbox/messageRules"
}

func (s *Service) outlookAccountID(ctx context.Context, token string) (string, error) {
	var account struct{ ID string }
	if err := s.outlookRequest(ctx, token, http.MethodGet, s.outlookRoot()+"?$select=id", nil, &account); err != nil {
		return "", err
	}
	if account.ID == "" || len(account.ID) > 256 {
		return "", &outlookGraphError{status: http.StatusBadGateway}
	}
	return account.ID, nil
}

func (s *Service) remoteOutlookRules(ctx context.Context, token string) ([]outlookRule, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	root := s.outlookRulesPath()
	rules := []outlookRule{}
	next := root
	for range 10 {
		var page outlookPage[outlookRule]
		if err := s.outlookRequest(ctx, token, http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		if page.Value == nil {
			break
		}
		rules = append(rules, *page.Value...)
		if page.Next == "" {
			sort.SliceStable(rules, func(i, j int) bool { return rules[i].Sequence < rules[j].Sequence })
			return rules, nil
		}
		if len(page.Next) > 16384 || page.Next == next || !outlookContinuation(root, page.Next) {
			break
		}
		next = page.Next
	}
	return nil, &outlookGraphError{status: http.StatusBadGateway}
}

func outlookRuleFailure(err error) error {
	var graph *outlookGraphError
	if errors.As(err, &graph) {
		switch graph.status {
		case http.StatusForbidden:
			return apperr.New(api.ErrorCodeOUTLOOKRULESACCESS, "Allow inbox rules to let HyPlug manage Outlook rules.")
		case http.StatusNotFound:
			return apperr.NotFound("Outlook rule")
		case http.StatusBadRequest:
			return apperr.Validation(apperr.FieldErrors{{Field: "rule", Message: "Outlook refused this rule. Check the conditions and folder."}})
		}
	}
	return apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, "Outlook could not confirm the rules request. Refresh the rules to see the latest state.")
}

func (s *Service) withOutlookRules(ctx context.Context, p authz.Principal, fn func(pgx.Tx, *store.Queries, string, string) error) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) || err == nil && box.Provider != ProviderMicrosoft {
			return outlookDisconnected()
		}
		if err != nil {
			return err
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		return fn(tx, q, box.ID, token)
	})
}

func outlookRuleToAPI(rule outlookRule, mailbox, folder string) api.OutlookRule {
	return api.OutlookRule{
		Id: base64.RawURLEncoding.EncodeToString([]byte(mailbox + ":" + rule.ID)), Name: rule.DisplayName, Enabled: rule.IsEnabled,
		ReadOnly: rule.IsReadOnly, HasError: rule.HasError, Destination: folder,
	}
}

func ruleDestination(kind string) bool { return kind == "archive" || kind == "other" }

func (s *Service) listOutlookRules(ctx context.Context, p authz.Principal) (api.OutlookRuleList, error) {
	out := api.OutlookRuleList{Items: []api.OutlookRule{}, Folders: []api.OutlookRuleFolder{}}
	var mailbox, token string
	var folders []store.EmailOutlookFolder
	err := s.withOutlookRules(ctx, p, func(_ pgx.Tx, q *store.Queries, id, access string) error {
		mailbox, token = id, access
		var err error
		folders, err = q.OutlookFolders(ctx, mailbox)
		return err
	})
	if err != nil {
		return out, err
	}
	// Reading the provider can be slow. Release the mailbox lock before the
	// read so the sync and writes may proceed. Returned rule IDs bind this
	// mailbox snapshot, so a stale list cannot mutate its replacement.
	rules, err := s.remoteOutlookRules(ctx, token)
	if err != nil {
		return out, outlookRuleFailure(err)
	}
	byRemoteID := make(map[string]store.EmailOutlookFolder, len(folders))
	names := make(map[string]string, len(folders))
	for _, folder := range folders {
		byRemoteID[folder.RemoteID] = folder
	}
	for _, folder := range folders {
		name := outlookRuleFolderPath(folder, byRemoteID)
		names[folder.RemoteID] = name
		if ruleDestination(folder.Kind) {
			out.Folders = append(out.Folders, api.OutlookRuleFolder{Id: folder.ID, Name: name})
		}
	}
	for _, rule := range rules {
		if !outlookGraphID.MatchString(rule.ID) {
			continue
		}
		destination := names[rule.Actions.MoveToFolder]
		if destination == "" && rule.Actions.MoveToFolder != "" {
			destination = "Outlook folder"
		}
		out.Items = append(out.Items, outlookRuleToAPI(rule, mailbox, destination))
	}
	return out, nil
}

func outlookRuleFolderPath(folder store.EmailOutlookFolder, byRemoteID map[string]store.EmailOutlookFolder) string {
	names := []string{folder.Name}
	seen := map[string]bool{folder.RemoteID: true}
	for folder.ParentID != nil {
		parent, found := byRemoteID[*folder.ParentID]
		if !found || seen[parent.RemoteID] {
			break
		}
		seen[parent.RemoteID] = true
		names = append(names, parent.Name)
		folder = parent
	}
	slices.Reverse(names)
	return strings.Join(names, " / ")
}

func (s *Service) createOutlookRule(ctx context.Context, p authz.Principal, in api.OutlookRuleInput) (api.OutlookRule, error) {
	out := api.OutlookRule{}
	name := strings.TrimSpace(in.Name)
	var conditions outlookRuleConditions
	if in.FromAddress != nil {
		var address outlookRuleAddress
		address.EmailAddress.Address = strings.TrimSpace(string(*in.FromAddress))
		if address.EmailAddress.Address != "" {
			conditions.FromAddresses = []outlookRuleAddress{address}
		}
	}
	if in.SubjectContains != nil {
		if subject := strings.TrimSpace(*in.SubjectContains); subject != "" {
			conditions.SubjectContains = []string{subject}
		}
	}
	var fields apperr.FieldErrors
	if name == "" {
		fields.Add("name", "enter a name")
	}
	if len(conditions.FromAddresses) == 0 && len(conditions.SubjectContains) == 0 {
		fields.Add("fromAddress", "enter a sender or subject text")
	}
	if err := fields.Err(); err != nil {
		return out, err
	}
	err := s.withOutlookRules(ctx, p, func(tx pgx.Tx, q *store.Queries, mailbox, token string) error {
		folder, err := q.OutlookFolder(ctx, store.OutlookFolderParams{MailboxID: mailbox, ID: in.FolderId})
		if database.IsNoRows(err) || err == nil && !ruleDestination(folder.Kind) {
			return apperr.NotFound("Outlook destination folder")
		}
		if err != nil {
			return err
		}
		rules, err := s.remoteOutlookRules(ctx, token)
		if err != nil {
			return outlookRuleFailure(err)
		}
		sequence := int32(1)
		for _, rule := range rules {
			if strings.EqualFold(rule.DisplayName, name) {
				return apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "an Outlook rule with this name already exists"}})
			}
			if rule.Sequence == math.MaxInt32 {
				return apperr.Validation(apperr.FieldErrors{{Field: "rule", Message: "reorder the existing rules in Outlook before adding another"}})
			}
			sequence = max(sequence, rule.Sequence+1)
		}
		input := struct {
			outlookRule
			Conditions outlookRuleConditions `json:"conditions"`
		}{
			outlookRule: outlookRule{DisplayName: name, Sequence: sequence, IsEnabled: true,
				Actions: outlookRuleActions{MoveToFolder: folder.RemoteID, StopProcessingRules: true}},
			Conditions: conditions,
		}
		var created outlookRule
		if err := s.outlookRequest(ctx, token, http.MethodPost, s.outlookRulesPath(), input, &created); err != nil {
			return outlookRuleFailure(err)
		}
		if !outlookGraphID.MatchString(created.ID) {
			return outlookRuleFailure(&outlookGraphError{status: http.StatusBadGateway})
		}
		out = outlookRuleToAPI(created, mailbox, folder.Name)
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.rule_created", EntityType: "outlook_rule", EntityID: out.Id})
	})
	return out, err
}

func (s *Service) changeOutlookRule(ctx context.Context, p authz.Principal, id string, enabled *bool) error {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(id)
	pinnedMailbox, remoteID, valid := strings.Cut(string(decoded), ":")
	if err != nil || !valid || !outlookGraphID.MatchString(remoteID) {
		return apperr.NotFound("Outlook rule")
	}
	return s.withOutlookRules(ctx, p, func(tx pgx.Tx, _ *store.Queries, mailbox, token string) error {
		if mailbox != pinnedMailbox {
			return apperr.NotFound("Outlook rule")
		}
		target := s.outlookRulesPath() + "/" + url.PathEscape(remoteID)
		var rule outlookRule
		if err := s.outlookRequest(ctx, token, http.MethodGet, target, nil, &rule); err != nil {
			return outlookRuleFailure(err)
		}
		if rule.ID != remoteID || rule.IsReadOnly {
			return apperr.Validation(apperr.FieldErrors{{Field: "rule", Message: "this Outlook rule cannot be changed in HyPlug"}})
		}
		if enabled == nil {
			if err := s.outlookRequest(ctx, token, http.MethodDelete, target, nil, nil); err != nil {
				return outlookRuleFailure(err)
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "outlook.rule_deleted", EntityType: "outlook_rule", EntityID: id})
		}
		if err := s.outlookRequest(ctx, token, http.MethodPatch, target, map[string]bool{"isEnabled": *enabled}, nil); err != nil {
			return outlookRuleFailure(err)
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.rule_updated", EntityType: "outlook_rule", EntityID: id})
	})
}
