package documents

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

	for name, data := range map[string][]byte{
		"plain text":          []byte("hello"),
		"a zip that is no CV": zipWith(t, "photo.jpg"),
		"empty":               nil,
		"old .doc":            {0xD0, 0xCF, 0x11, 0xE0},
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
