package filekind

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rtfCV = `{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fswiss Arial;}{\f1 Times New Roman;}}{\colortbl;\red0\green0\blue0;}
{\*\generator Riched20 10.0;}{\info{\author Recruiter}}\viewkind4\uc1\pard\f0\fs22 Ren\'e9e Tan\par
Senior Accountant \endash  Kuala Lumpur\par
\bullet\tab SAP FICO, MFRS\line Caf\u233?\u8217?s month-end\par
{\field{\*\fldinst HYPERLINK "https://x.example"}{\fldrslt linkedin.com/in/renee}}\par
}`

func odt(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range map[string]string{"mimetype": ODT, "content.xml": content} {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

const odtContentXML = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
<office:automatic-styles><style:style xmlns:style="x">Not text</style:style></office:automatic-styles>
<office:body><office:text>
<text:h>Siti Aisyah</text:h><text:p>Auditor<text:tab/>KPMG<text:s text:c="2"/>2019</text:p>
<text:list><text:list-item><text:p>Statutory audit</text:p></text:list-item></text:list>
<table:table><table:table-row><table:table-cell><text:p>SAP</text:p></table:table-cell><table:table-cell><text:p>5 years</text:p></table:table-cell></table:table-row></table:table>
</office:text></office:body></office:document-content>`

func TestFilesAreToldByTheirBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"cv.pdf", []byte("%PDF-1.7 ..."), PDF},
		{"cv.doc", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0}, DOC},
		{"cv.rtf", []byte(rtfCV), RTF},
		{"cv.odt", odt(t, odtContentXML), ODT},
		{"photo.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}, JPEG},
		{"scan.png", []byte("\x89PNG\r\n\x1a\nxxxx"), PNG},
		{"scan.tif", []byte("II*\x00xxxx"), TIFF},
		{"scan.webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), WEBP},
		{"cv.txt", []byte("Siti Aisyah\nAuditor"), TXT},
		{"cv.pdf.txt", []byte("Siti"), TXT},
		{"renamed.docx", []byte("%PDF-1.4"), PDF}, // the bytes win over the name
	}
	for _, c := range cases {
		got, ok := Detect(c.name, c.data)
		assert.True(t, ok, c.name)
		assert.Equal(t, c.want, got, c.name)
	}
	for name, data := range map[string][]byte{
		"cv.exe":   []byte("MZ\x90\x00"),
		"notes":    []byte("plain words without a .txt name"),
		"bin.txt":  {0x00, 0x00, 0x01, 0x02, 0x00, 0x00},
		"x.zip":    []byte("PK\x03\x04garbage"),
		"tiny.bmp": []byte("BM"),
	} {
		_, ok := Detect(name, data)
		assert.False(t, ok, name)
	}
}

func TestRTFIsReadFollowingTheFormat(t *testing.T) {
	text, err := Text(RTF, []byte(rtfCV))
	require.NoError(t, err)
	assert.Equal(t, "Renée Tan\nSenior Accountant – Kuala Lumpur\n•\tSAP FICO, MFRS\nCafé’s month-end\nlinkedin.com/in/renee", text)
	assert.NotContains(t, text, "Arial", "the font table is not text")
	assert.NotContains(t, text, "Riched20")
	assert.NotContains(t, text, "HYPERLINK")
}

func TestODTIsReadByParagraph(t *testing.T) {
	text, err := Text(ODT, odt(t, odtContentXML))
	require.NoError(t, err)
	assert.Equal(t, "Siti Aisyah\nAuditor\tKPMG  2019\n• Statutory audit\nSAP\n\t5 years", text)
	assert.NotContains(t, text, "Not text")
}

func TestTextFilesInAnyCommonEncoding(t *testing.T) {
	for name, data := range map[string][]byte{
		"utf-8":          []byte("Renée – CV"),
		"utf-8 with BOM": append([]byte{0xEF, 0xBB, 0xBF}, []byte("Renée – CV")...),
		"utf-16 LE BOM":  {0xFF, 0xFE, 'R', 0, 'e', 0, 'n', 0, 0xE9, 0, 'e', 0, ' ', 0, 0x13, 0x20, ' ', 0, 'C', 0, 'V', 0},
		"windows-1252":   {'R', 'e', 'n', 0xE9, 'e', ' ', 0x96, ' ', 'C', 'V'},
	} {
		text, err := Text(TXT, data)
		require.NoError(t, err, name)
		assert.Equal(t, "Renée – CV", text, name)
	}
	_, err := Text(TXT, []byte("   \n\n  "))
	assert.ErrorIs(t, err, ErrNoText)
}

func TestAHostileODTIsRefused(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("mimetype")
	require.NoError(t, err)
	_, _ = f.Write([]byte(ODT))
	f, err = w.Create("content.xml")
	require.NoError(t, err)
	_, _ = f.Write(bytes.Repeat([]byte("a"), maxODTContent+10)) // compresses to almost nothing
	require.NoError(t, w.Close())
	_, err = Text(ODT, buf.Bytes())
	assert.ErrorContains(t, err, "too large")
}

func TestRTFInOtherCodePagesWithPicturesAndLooseDestinations(t *testing.T) {
	text, err := Text(RTF, []byte(`{\rtf1\ansi\ansicpg936{\fonttbl{\f0 SimSun;}} \'b3\'c2\'ce\'b0\'c3\'f7\par Auditor\par}`))
	require.NoError(t, err)
	assert.Equal(t, "陈伟明\nAuditor", text, "GBK, as the file says")

	pic := append([]byte(`{\rtf1 Siti\par{\pict\pngblip\bin5 `), []byte("}{\x00}\xff")...)
	pic = append(pic, []byte(`}Auditor\par}`)...)
	text, err = Text(RTF, pic)
	require.NoError(t, err)
	assert.Equal(t, "Siti\nAuditor", text, "binary picture bytes are skipped, braces and all")

	text, err = Text(RTF, []byte(`{\rtf1{ \*\generator Msftedit;}Siti\par}`))
	require.NoError(t, err)
	assert.Equal(t, "Siti", text, "no stray * from a destination marker after a space")
}

func TestAnODTPageHeaderIsRead(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"mimetype":    ODT,
		"content.xml": odtContentXML,
		"styles.xml": `<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
			`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0">` +
			`<office:styles><style:style style:name="P1"/></office:styles><office:master-styles><style:master-page>` +
			`<style:header><text:p>siti@example.com · +60 12 345 6789</text:p></style:header></style:master-page></office:master-styles></office:document-styles>`,
	} {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, _ = f.Write([]byte(body))
	}
	require.NoError(t, w.Close())
	text, err := Text(ODT, buf.Bytes())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(text, "siti@example.com · +60 12 345 6789\nSiti Aisyah"), text)
}

func TestATextFileStartingWithBMIsNotAPicture(t *testing.T) {
	got, ok := Detect("cv.txt", []byte("BMW Group Malaysia, Data Engineer since 2019, Kuala Lumpur"))
	assert.True(t, ok)
	assert.Equal(t, TXT, got)
	bmp := make([]byte, 60)
	copy(bmp, "BM")
	bmp[14] = 40 // a BITMAPINFOHEADER
	got, _ = Detect("scan.bmp", bmp)
	assert.Equal(t, BMP, got)
}
