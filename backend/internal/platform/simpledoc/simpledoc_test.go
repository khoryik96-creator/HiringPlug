package simpledoc

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromText(t *testing.T) {
	got := FromText("About us\nWe hire.\n\n- One\n• Two\n* Three\nAfter")
	assert.Equal(t, Doc{{Paragraph, "About us\nWe hire."}, {Bullet, "One"}, {Bullet, "Two"}, {Bullet, "Three"}, {Paragraph, "After"}}, got)
}

func TestDocxAndPDF(t *testing.T) {
	doc := Doc{{Title, "Pengurus Kewangan & <Finance>"}, {Muted, "KL · RM 8,000"}, {Paragraph, "Line one\nline two"}, {Bullet, "Close the books – monthly"}}
	data, err := doc.Docx()
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var body string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			r, err := f.Open()
			require.NoError(t, err)
			b, _ := io.ReadAll(r)
			body = string(b)
		}
	}
	assert.Contains(t, body, "Pengurus Kewangan &amp; &lt;Finance&gt;", "text is escaped")
	assert.Contains(t, body, "<w:br/>")

	pdf, err := doc.PDF()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(pdf, []byte("%PDF-")))
}
