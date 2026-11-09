package simpledoc

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromText(t *testing.T) {
	got := FromText("About us\nWe hire.\n\n- One\n• Two\n* Three\nAfter")
	assert.Equal(t, Doc{{Kind: Paragraph, Text: "About us\nWe hire."}, {Kind: Bullet, Text: "One"}, {Kind: Bullet, Text: "Two"}, {Kind: Bullet, Text: "Three"}, {Kind: Paragraph, Text: "After"}}, got)
}

func TestDocxAndPDF(t *testing.T) {
	doc := Doc{{Kind: Title, Text: "Pengurus Kewangan & <Finance>"}, {Kind: Muted, Text: "KL · RM 8,000"}, {Kind: Paragraph, Text: "Line one\nline two"}, {Kind: Bullet, Text: "Close the books – monthly"}}
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

func TestTables(t *testing.T) {
	doc := Doc{
		{Kind: Heading, Text: "Benefits"},
		{Kind: HeaderRow, Cells: []string{"Item", "Value", "Remarks"}},
		{Kind: Row, Cells: []string{"Annual leave", "18 days", "21 after 3 years\nPro-rated"}},
		{Kind: Row, Cells: []string{"Medical & insurance", "RM 5,000 a year", ""}},
		{Kind: Paragraph, Text: "Between tables"},
		{Kind: HeaderRow, Cells: []string{"Item", "Applies"}},
	}
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
	assert.Equal(t, 2, strings.Count(body, "<w:tbl>"), "rows next to each other make one table")
	assert.Equal(t, 4, strings.Count(body, "<w:tr>"))
	assert.Equal(t, 5, strings.Count(body, `<w:gridCol `), "three columns, then two")
	assert.Contains(t, body, "Medical &amp; insurance")
	assert.Contains(t, body, `w:fill="F3F4F6"`, "the header row is shaded")

	// Many rows run onto more pages.
	long := Doc{{Kind: HeaderRow, Cells: []string{"Item", "Value"}}}
	for range 80 {
		long = append(long, Block{Kind: Row, Cells: []string{"Item", "A value that wraps onto a second line in its narrow column, to be sure"}})
	}
	pdf, err := long.PDF()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(pdf, []byte("%PDF-")))
	assert.Greater(t, bytes.Count(pdf, []byte("/Type /Page\n")), 1, "more than one page")
}
