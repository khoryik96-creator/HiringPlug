package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func demoExtraction(r *http.Request) (*http.Response, error) {
	p := strings.TrimPrefix(r.URL.Path, "/v2")
	code := 200
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	candidate := map[string]any{"candidateId": 7, "firstName": "Aina", "lastName": "Rahman", "skillTags": []string{"QA", "Playwright"}, "custom": []any{map[string]any{"fieldId": 3, "name": "Original label", "value": "Keep unchanged"}}, "employment": map[string]any{"current": map[string]any{"position": "QA Engineer", "employer": "Sample Fintech"}}, "notes": "Synthetic record only"}
	company := map[string]any{"companyId": 9, "name": "Sample Fintech", "addresses": []any{map[string]any{"addressId": 1, "city": "Kuala Lumpur"}}}
	note := map[string]any{"noteId": "11111111-1111-4111-8111-111111111111", "subject": "Screening call", "text": "Original synthetic notes. Keep this wording.", "candidates": []any{map[string]any{"candidateId": 7}}, "attachments": []any{map[string]any{"attachmentId": "22222222-2222-4222-8222-222222222222", "fileName": "call-notes.txt"}}, "createdBy": map[string]any{"userId": 1}}
	items := []any{}
	var data any
	switch p {
	case "/candidates":
		items = []any{candidate}
	case "/companies":
		items = []any{company}
	case "/notes":
		items = []any{note}
	case "/candidates/attachments":
		items = []any{map[string]any{"candidateId": 7, "attachmentId": json.Number("9007199254740993"), "fileName": "Aina-original.txt", "type": "Resume"}}
	case "/jobs":
		if r.URL.Query().Get("Active") == "false" {
			items = []any{map[string]any{"jobId": 11, "title": "Archived sample role", "company": company, "status": map[string]any{"statusId": 2, "name": "Closed"}}}
		}
	case "/folders":
		items = []any{map[string]any{"folderId": 20, "name": "QA shortlist", "entityIDs": map[string]any{"candidates": []int{7}}}}
	case "/users":
		items = []any{map[string]any{"userId": 1, "firstName": "Sample", "lastName": "Recruiter"}}
	case "/candidates/7":
		data = candidate
	case "/companies/9":
		data = company
	case "/notes/11111111-1111-4111-8111-111111111111":
		data = note
	case "/folders/20":
		data = map[string]any{"folderId": 20, "name": "QA shortlist", "entityIDs": map[string]any{"candidates": []int{7}}}
	case "/jobs/11":
		data = map[string]any{"jobId": 11, "title": "Archived sample role", "company": company, "active": false}
	case "/users/1", "/users/current":
		data = map[string]any{"userId": 1, "accountName": "Sample agency — DEMO"}
	case "/opportunities":
		code = 403
	}
	if p == "/candidates/7/attachments/9007199254740993" || strings.HasPrefix(p, "/notes/11111111-1111-4111-8111-111111111111/attachments/") {
		header.Set("Content-Type", "text/plain")
		header.Set("Content-Disposition", `attachment; filename="synthetic-original.txt"`)
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader("Synthetic original document.\r\nNo real candidate data.\r\n")), Request: r}, nil
	}
	if strings.HasSuffix(p, "/photo") || strings.HasSuffix(p, "/logo") {
		code = 404
	}
	if data == nil {
		data = map[string]any{"items": items, "totalCount": len(items)}
	}
	raw, _ := json.Marshal(data)
	return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
}
