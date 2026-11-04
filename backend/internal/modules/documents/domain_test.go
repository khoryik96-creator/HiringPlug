package documents

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func zipWith(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	_, err := z.Create(name)
	assert.NoError(t, err)
	assert.NoError(t, z.Close())
	return buf.Bytes()
}

func TestDetectContentType(t *testing.T) {
	got, ok := DetectContentType([]byte("%PDF-1.7 ..."))
	assert.True(t, ok)
	assert.Equal(t, ContentTypePDF, got)

	got, ok = DetectContentType(zipWith(t, "word/document.xml"))
	assert.True(t, ok)
	assert.Equal(t, ContentTypeDOCX, got)

	got, ok = DetectContentType([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0})
	assert.True(t, ok)
	assert.Equal(t, ContentTypeDOC, got)

	for name, data := range map[string][]byte{
		"plain text":           []byte("hello"),
		"a zip that is no CV":  zipWith(t, "photo.jpg"),
		"empty":                nil,
		"a cut-off OLE header": {0xD0, 0xCF, 0x11, 0xE0},
	} {
		_, ok := DetectContentType(data)
		assert.False(t, ok, name)
	}
}

func TestCleanFileName(t *testing.T) {
	assert.Equal(t, "cv.pdf", CleanFileName(`C:\Users\me\cv.pdf`))
	assert.Equal(t, "cv.pdf", CleanFileName("../../etc/cv.pdf"))
	assert.Equal(t, "my cv.pdf", CleanFileName("my\x00 cv.pdf\n"))
	assert.Equal(t, "upload", CleanFileName(".."))
	assert.Len(t, []rune(CleanFileName(strings.Repeat("é", 300))), 255)
}

func TestCleanExtractedTextRemovesNUL(t *testing.T) {
	assert.Equal(t, "ab", CleanExtractedText("a\x00b"))
}

func TestValuesMustBeReadableFromTheirQuote(t *testing.T) {
	for _, c := range []struct {
		kind, value, quote string
		ok                 bool
	}{
		{FieldText, "Siti Aisyah", "SITI AISYAH", true},
		{FieldText, "Siti Binti Ahmad", "SITI BINT] AHMAD", true}, // OCR's "]" for a final I
		{FieldText, "Excel", "Excel (Advanced)", true},
		{FieldText, "+60 12-345 6789", "+60 12-345 6789", true},
		{FieldText, "Python", "SAP", false},
		{FieldText, "", "SAP", false},
		{FieldURL, "https://www.linkedin.com/in/siti", "linkedin.com/in/siti", true},
		{FieldURL, "https://evil.example/siti", "linkedin.com/in/siti", false},
		{FieldCountry, "MY", "Nationality: Malaysian", true},
		{FieldCountry, "Malaysia", "Nationality: Malaysian", false},
		{FieldNoticePeriod, "1 month", "Notice period: one month", true},
		{FieldNoticePeriod, "forever", "Notice period: forever", false},
	} {
		assert.Equal(t, c.ok, ValueFromQuote(c.kind, c.value, c.quote), "%s %q from %q", c.kind, c.value, c.quote)
	}
}

func TestLengthLimitsCountCharactersNotBytes(t *testing.T) {
	name := strings.Repeat("陈", 200) // 200 characters, 600 bytes: within the 300-character limit
	got := groundProfile(name, api.ParsedProfile{FullName: nullable.NewNullableWithValue(api.SourcedText{Value: name, Quote: name})})
	assert.Equal(t, 0, got.Dropped)
	assert.Equal(t, name, got.FullName.MustGet().Value)
}
