package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func sumHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestTheSameCVIsNeverUploadedTwice(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Huda"})
	cv, newer := pdfWith("Nurul 2025"), pdfWith("Nurul 2026")
	docs := "/v1/candidates/" + nurul.Id + "/documents"

	res := a.upload(docs, token, "original", "Nurul Huda CV.pdf", cv)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var first api.Document
	res.JSON(&first)

	t.Run("exactly the same file again is refused", func(t *testing.T) {
		res := a.upload(docs, token, "original", "Copy of CV.pdf", cv)
		require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
		e := res.Error()
		assert.Equal(t, api.ErrorCodeDOCUMENTDUPLICATE, e.Error.Code)
		assert.Equal(t, first.Id, (*e.Error.Details)["existingDocumentId"])
	})

	t.Run("before uploading: same content to skip, same name to ask", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/document-matches", token, map[string]any{"files": []map[string]any{
			{"fileName": "renamed.pdf", "sha256": sumHex(cv)},
			{"fileName": "nurul huda cv.PDF", "sha256": sumHex(newer)},
			{"fileName": "Someone else.pdf", "sha256": sumHex(pdfWith("other"))},
		}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var got api.DocumentMatchList
		res.JSON(&got)
		require.Len(t, got.Items, 3)
		same := got.Items[0].SameContent.MustGet()
		assert.Equal(t, first.Id, same.DocumentId)
		assert.Equal(t, "Nurul Huda", same.CandidateName)
		assert.True(t, got.Items[1].SameContent.IsNull())
		require.Len(t, got.Items[1].SameName, 1, "the same name, whatever the case, other content")
		assert.Equal(t, "Nurul Huda CV.pdf", got.Items[1].SameName[0].FileName)
		assert.True(t, got.Items[2].SameContent.IsNull())
		assert.Empty(t, got.Items[2].SameName)

		other := a.createCandidate(token, map[string]any{"firstName": "Siti"})
		res = a.request(http.MethodPost, "/v1/document-matches", token, map[string]any{"candidateId": other.Id,
			"files": []map[string]any{{"fileName": "Nurul Huda CV.pdf", "sha256": sumHex(cv)}}})
		res.JSON(&got)
		assert.True(t, got.Items[0].SameContent.IsNull(), "with candidateId, only that candidate's CVs")
	})

	t.Run("an import skips a file already in HiringPlug", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/cv-imports", token, map[string]any{"source": "linkedin"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var imp api.CvImport
		res.JSON(&imp)
		res = a.upload("/v1/cv-imports/"+imp.Id+"/files", token, "", "Profile (1).pdf", cv)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var item api.CvImportItem
		res.JSON(&item)
		assert.Equal(t, api.CvImportItemStatusDuplicate, item.Status)
		assert.Equal(t, nurul.Id, item.CandidateId.MustGet(), "linked to whose CV it is")
		assert.Contains(t, item.Error.MustGet(), "Nurul Huda")
		assert.Equal(t, 1, a.cvImport(token, imp.Id).Counts.Duplicate)
	})
}
