package app_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func odtFile(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"mimetype", "application/vnd.oasis.opendocument.text"},
		{"content.xml", `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
			`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"><office:body><office:text>` + body +
			`</office:text></office:body></office:document-content>`},
	} {
		fw, err := w.Create(f.name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(f.body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestCVsComeInMoreFormatsAsInCVStudio(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	aiConn := aiConnFor(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	docs := "/v1/candidates/" + c.Id + "/documents"
	upload := func(name string, data []byte) api.Document {
		res := a.upload(docs, token, "original", name, data)
		require.Equal(t, http.StatusCreated, res.Status, "%s: %s", name, res.Body)
		var d api.Document
		res.JSON(&d)
		return d
	}
	text := func(id string) string {
		var d api.Document
		a.request(http.MethodGet, "/v1/documents/"+id, token, nil).JSON(&d)
		return d.ExtractedText.MustGet()
	}

	for name, data := range map[string][]byte{
		"cv.txt": []byte("Siti Aisyah\nSenior Auditor, SAP FICO"),
		"cv.rtf": []byte(`{\rtf1\ansi{\fonttbl{\f0 Arial;}}\f0 Siti Aisyah\par Senior Auditor, SAP FICO\par}`),
		"cv.odt": odtFile(t, `<text:p>Siti Aisyah</text:p><text:p>Senior Auditor, SAP FICO</text:p>`),
	} {
		d := upload(name, data)
		assert.Equal(t, api.DocumentStatusExtracted, d.Status, "%s is read at once, here", name)
		assert.Equal(t, "Siti Aisyah\nSenior Auditor, SAP FICO", text(d.Id), name)
		parse := claimAIJobFor(t, aiConn, d.Id)
		assert.Equal(t, string(api.JobKindDocumentParseCv), parse.Kind, "%s: its details are read next", name)
		var asked api.DocumentParseCvJob
		require.NoError(t, json.Unmarshal(parse.Payload, &asked))
		assert.Equal(t, name, *asked.FileName, "the file's name helps find the candidate's name")
	}

	photo := upload("cv-photo.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'})
	assert.Equal(t, api.DocumentStatusQueued, photo.Status, "a picture is read with OCR by the AI worker")
	read := claimAIJobFor(t, aiConn, photo.Id)
	assert.Equal(t, string(api.JobKindDocumentExtractText), read.Kind)
	var asked api.DocumentExtractTextJob
	require.NoError(t, json.Unmarshal(read.Payload, &asked))
	assert.Equal(t, "image/jpeg", asked.ContentType)

	res := a.upload(docs, token, "original", "empty.rtf", []byte(`{\rtf1{\fonttbl{\f0 Arial;}}}`))
	assert.Equal(t, http.StatusUnsupportedMediaType, res.Status, "nothing to read")
	res = a.upload(docs, token, "original", "slides.pptx", []byte("PK\x03\x04garbage"))
	assert.Equal(t, http.StatusUnsupportedMediaType, res.Status)
	assert.Contains(t, string(res.Body), "picture of the CV")

	jd := a.upload("/v1/jd-files", token, "", "Auditor JD.rtf", []byte(`{\rtf1 Senior Auditor\par Requirements: ACCA, SAP FICO\par}`))
	require.Equal(t, http.StatusCreated, jd.Status, "%s", jd.Body)
	var f api.JdFile
	jd.JSON(&f)
	assert.Equal(t, api.JdFileStatusRead, f.Status, "a JD in RTF is read at once too")
}
