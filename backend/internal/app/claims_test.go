package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A synthetic receipt photo: a PNG header is enough for the file type, the
// "reading" is played by the test.
var receiptPNG = append([]byte("\x89PNG\r\n\x1a\n"), []byte("synthetic receipt 1")...)

func receiptBytes(n string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte("synthetic receipt "+n)...)
}

func (a *testApp) uploadReceipt(token, name string, data []byte) api.Claim {
	a.t.Helper()
	res := a.upload("/v1/claims", token, "", name, data)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var c api.Claim
	res.JSON(&c)
	return c
}

func (a *testApp) claim(token, id string) api.Claim {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/claims/"+id, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var c api.Claim
	res.JSON(&c)
	return c
}

func (a *testApp) patchClaim(token string, c api.Claim, body map[string]any, want int) api.Claim {
	a.t.Helper()
	body["version"] = c.Version
	res := a.request(http.MethodPatch, "/v1/claims/"+c.Id, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.Claim
	if want == http.StatusOK {
		res.JSON(&out)
	}
	return out
}

func daysAgo(n int) openapi_types.Date {
	return openapi_types.Date{Time: time.Now().UTC().AddDate(0, 0, -n).Truncate(24 * time.Hour)}
}

// readReceipt plays the Python worker for a claim's receipt.
func readReceipt(t *testing.T, a *testApp, conn *pgx.Conn, claimID string, result api.ClaimReceiptReadJob) api.ClaimReadReceiptJob {
	t.Helper()
	job := claimAIJobWhere(t, conn, "claimId", claimID)
	require.Equal(t, string(api.JobKindClaimReadReceipt), job.Kind)
	var asked api.ClaimReadReceiptJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	result.ClaimId = claimID
	answerAIJob(t, a, conn, job, api.JobKindClaimReceiptRead, result)
	return asked
}

func receiptRead(date openapi_types.Date, amount, currency, merchant, receiptType string) api.ClaimReceiptReadJob {
	return api.ClaimReceiptReadJob{
		Status: api.ClaimReadStatusRead, ReceiptDate: nullable.NewNullableWithValue(date), Amount: nullable.NewNullableWithValue(amount),
		Currency: nullable.NewNullableWithValue(currency), Merchant: nullable.NewNullableWithValue(merchant),
		ReceiptType: nullable.NewNullableWithValue(receiptType), Extractor: "tesseract@test", ErrorMessage: nullable.NewNullNullable[string](),
	}
}

func claimFieldNames(fs []api.ClaimField) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, string(f))
	}
	return out
}

func TestAReceiptIsReadIntoFieldsThePersonCanEdit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	c := a.uploadReceipt(token, `C:\photos\grab receipt.png`, receiptPNG)
	assert.Equal(t, api.ClaimStatusDraft, c.Status)
	assert.Equal(t, api.ClaimOcrStatusReading, c.OcrStatus)
	assert.Equal(t, "grab receipt.png", c.FileName)
	assert.Equal(t, "MYR", c.Currency)
	assert.Equal(t, []string{"receiptDate", "amount", "receiptType", "claimType", "purpose"}, claimFieldNames(c.Missing))

	asked := readReceipt(t, a, aiConn, c.Id, receiptRead(daysAgo(3), "23.5", "MYR", "Grab", "grab_fare"))
	assert.Contains(t, asked.ReceiptTypes, api.ClaimReceiptTypeOption{Key: "grab_fare", Label: "Grab / e-hailing fare"},
		"the reader chooses among the company's receipt types")
	assert.Equal(t, "image/png", asked.ContentType)

	c = a.claim(token, c.Id)
	assert.Equal(t, api.ClaimOcrStatusDone, c.OcrStatus)
	assert.Equal(t, daysAgo(3), c.ReceiptDate.MustGet())
	assert.Equal(t, "23.50", c.Amount.MustGet())
	assert.Equal(t, "Grab", c.Merchant.MustGet())
	assert.Equal(t, "grab_fare", c.ReceiptType.MustGet())
	assert.Equal(t, "travel", c.ClaimType.MustGet(), "a fare suggests a travel claim")
	assert.ElementsMatch(t, []string{"receiptDate", "amount", "merchant", "receiptType", "claimType"}, claimFieldNames(c.FromReceipt))
	assert.Equal(t, []string{"purpose"}, claimFieldNames(c.Missing))

	// Changing a value: it is the person's now, no longer a suggestion.
	c = a.patchClaim(token, c, map[string]any{"amount": "25.00", "purpose": "  Meeting   Acme in KLCC "}, http.StatusOK)
	assert.Equal(t, "25.00", c.Amount.MustGet())
	assert.Equal(t, "Meeting Acme in KLCC", c.Purpose.MustGet())
	assert.NotContains(t, claimFieldNames(c.FromReceipt), "amount")
	assert.Contains(t, claimFieldNames(c.FromReceipt), "merchant")
	assert.Empty(t, c.Missing)

	// A reading that arrives after the person typed never replaces their values,
	// and what is not valid (a day ahead, a type the company lacks) is dropped.
	typed := a.uploadReceipt(token, "lunch.png", receiptBytes("2"))
	typed = a.patchClaim(token, typed, map[string]any{"amount": "40.00", "merchant": "Nasi Kandar Pelita"}, http.StatusOK)
	readReceipt(t, a, aiConn, typed.Id, receiptRead(daysAgo(-5), "99.90", "SGD", "PELITA NASI KANDAR SDN BHD", "yacht"))
	typed = a.claim(token, typed.Id)
	assert.Equal(t, "40.00", typed.Amount.MustGet())
	assert.Equal(t, "Nasi Kandar Pelita", typed.Merchant.MustGet())
	assert.Equal(t, "MYR", typed.Currency, "a claim already edited keeps its currency")
	assert.False(t, typed.ReceiptDate.IsSpecified() && !typed.ReceiptDate.IsNull(), "a receipt date ahead is dropped")
	assert.True(t, typed.ReceiptType.IsNull())
	assert.Empty(t, claimFieldNames(typed.FromReceipt))

	// Nothing could be read: say so, the person fills it in.
	blank := a.uploadReceipt(token, "blurry.png", receiptBytes("3"))
	readReceipt(t, a, aiConn, blank.Id, api.ClaimReceiptReadJob{Status: api.ClaimReadStatusUnreadable, Extractor: "tesseract@test",
		ReceiptDate: nullable.NewNullNullable[openapi_types.Date](), Amount: nullable.NewNullNullable[string](), Currency: nullable.NewNullNullable[string](),
		Merchant: nullable.NewNullNullable[string](), ReceiptType: nullable.NewNullNullable[string](),
		ErrorMessage: nullable.NewNullableWithValue("No text could be read from this photo.")})
	blank = a.claim(token, blank.Id)
	assert.Equal(t, api.ClaimOcrStatusFailed, blank.OcrStatus)
	assert.Equal(t, "No text could be read from this photo.", blank.OcrError.MustGet())

	// Never read (the reader is down, or its job died): after a while the
	// claim is the person's to fill in; a late reading fills what is empty.
	stuck := a.uploadReceipt(token, "taxi.png", receiptBytes("4"))
	_, err := testutil.Current.Admin.Exec(t.Context(), `UPDATE claim SET created_at = now() - interval '11 minutes' WHERE id = $1`, stuck.Id)
	require.NoError(t, err)
	stuck = a.claim(token, stuck.Id)
	assert.Equal(t, api.ClaimOcrStatusFailed, stuck.OcrStatus)
	assert.Equal(t, "The receipt could not be read in time: fill it in.", stuck.OcrError.MustGet())
	stuck = a.patchClaim(token, stuck, map[string]any{"amount": "18.00"}, http.StatusOK)
	readReceipt(t, a, aiConn, stuck.Id, receiptRead(daysAgo(1), "99.00", "MYR", "Public Cab", "grab_fare"))
	stuck = a.claim(token, stuck.Id)
	assert.Equal(t, api.ClaimOcrStatusDone, stuck.OcrStatus)
	assert.Equal(t, "18.00", stuck.Amount.MustGet())
	assert.Equal(t, "Public Cab", stuck.Merchant.MustGet())
}

func TestClaimFieldsAreChecked(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.uploadReceipt(token, "receipt.png", receiptPNG)
	for field, value := range map[string]any{
		"amount": "0", "receiptDate": time.Now().AddDate(0, 0, 3).Format(time.DateOnly), "receiptType": "yacht",
		"claimType": "bonus", "currency": "ringgit",
	} {
		res := a.request(http.MethodPatch, "/v1/claims/"+c.Id, token, map[string]any{"version": c.Version, field: value})
		assert.Equal(t, http.StatusBadRequest, res.Status, "%s: %s", field, res.Body)
	}
	c = a.patchClaim(token, c, map[string]any{"receiptType": "food", "claimType": "client_entertainment", "currency": "SGD"}, http.StatusOK)
	assert.Equal(t, "SGD", c.Currency)
	c = a.patchClaim(token, c, map[string]any{"receiptType": nil}, http.StatusOK)
	assert.True(t, c.ReceiptType.IsNull(), "null clears a field")
	res := a.request(http.MethodPatch, "/v1/claims/"+c.Id, token, map[string]any{"version": 1, "purpose": "stale"})
	assert.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, api.ErrorCodeCONFLICTSTALEVERSION, res.Error().Error.Code)

	// The same receipt twice is refused; other files are not receipts.
	res = a.upload("/v1/claims", token, "", "again.png", receiptPNG)
	require.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, api.ErrorCodeCLAIMDUPLICATE, res.Error().Error.Code)
	res = a.upload("/v1/claims", token, "", "notes.txt", []byte("Lunch 25.00"))
	assert.Equal(t, http.StatusUnsupportedMediaType, res.Status)
	// Deleted, the receipt can be claimed again.
	require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/claims/"+c.Id, token, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/claims/"+c.Id, token, nil).Status)
	a.uploadReceipt(token, "again.png", receiptPNG)
}

func TestClaimsAreTaggedToClientsAndCandidatesThePersonMaySee(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	acme := a.createCompany(recruiter, map[string]any{"name": "Acme Berhad"})
	hr := a.createContact(recruiter, map[string]any{"firstName": "Farah", "lastName": "Idris", "companyId": acme.Id})
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah", "currentTitle": "QA Engineer"})

	c := a.uploadReceipt(recruiter, "receipt.png", receiptPNG)
	c = a.patchClaim(recruiter, c, map[string]any{"link": map[string]any{"kind": "contact", "id": hr.Id}}, http.StatusOK)
	link := c.Link.MustGet()
	assert.Equal(t, api.ClaimLinkKindContact, link.Kind)
	assert.Equal(t, "Farah Idris", link.Name.MustGet())
	assert.Equal(t, "Acme Berhad", link.Detail.MustGet(), "a contact shows their company")
	c = a.patchClaim(recruiter, c, map[string]any{"link": map[string]any{"kind": "candidate", "id": siti.Id}}, http.StatusOK)
	assert.Equal(t, "Siti Aminah", c.Link.MustGet().Name.MustGet())
	assert.Equal(t, "QA Engineer", c.Link.MustGet().Detail.MustGet())
	assert.Equal(t, siti.Id, a.claim(admin, c.Id).Link.MustGet().Id, "an approver sees the tag")

	// A record of the wrong kind, of another company or made up is refused.
	other := testutil.Current.NewTenant(t, api.RoleRecruiter)
	elsewhere := a.createCompany(other.Users[api.RoleRecruiter].Token, map[string]any{"name": "Elsewhere Sdn Bhd"})
	for _, l := range []map[string]any{
		{"kind": "company", "id": siti.Id}, {"kind": "company", "id": elsewhere.Id}, {"kind": "candidate", "id": "cand_01jbz6w0m0k8q9x2r3s4t5v6w7"},
	} {
		res := a.request(http.MethodPatch, "/v1/claims/"+c.Id, recruiter, map[string]any{"version": c.Version, "link": l})
		assert.Equal(t, http.StatusBadRequest, res.Status, "%v: %s", l, res.Body)
	}
	c = a.patchClaim(recruiter, c, map[string]any{"link": nil}, http.StatusOK)
	assert.True(t, c.Link.IsNull())

	// A merged duplicate's claims follow the candidate kept.
	dup := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah B"})
	c = a.patchClaim(recruiter, c, map[string]any{"link": map[string]any{"kind": "candidate", "id": dup.Id}}, http.StatusOK)
	res := a.request(http.MethodPost, "/v1/candidates/"+siti.Id+"/merge", admin, map[string]any{"duplicateId": dup.Id, "version": siti.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, siti.Id, a.claim(recruiter, c.Id).Link.MustGet().Id)
}

func TestTheSameReceiptSentTwiceAtOnceIsOneClaim(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	first := a.uploadReceipt(token, "first.png", receiptBytes("first"))
	twice := receiptBytes("twice")
	sum := sha256.Sum256(twice)

	// The same receipt is on its way in another upload: written, not yet
	// committed, so this upload finds nothing and waits on it.
	tx, err := testutil.Current.Admin.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(t.Context(), `UPDATE claim SET sha256 = $1 WHERE id = $2`, hex.EncodeToString(sum[:]), first.Id)
	require.NoError(t, err)
	done := make(chan *response, 1)
	go func() { done <- a.upload("/v1/claims", token, "", "twice.png", twice) }()
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, tx.Commit(t.Context()))
	res := <-done
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeCLAIMDUPLICATE, res.Error().Error.Code)
	require.NotNil(t, res.Error().Error.Details)
	assert.Equal(t, first.Id, (*res.Error().Error.Details)["existingClaimId"])
}
