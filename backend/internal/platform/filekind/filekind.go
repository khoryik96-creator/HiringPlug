// Package filekind tells what an uploaded CV or job description is from its
// bytes (a name or a browser's claimed type is never trusted), and reads the
// text of the plain formats itself: text files, RTF and OpenDocument text.
//
// What it accepts follows CV Studio: PDF, Word (.docx and old .doc), .txt,
// .rtf, .odt, and pictures of a CV (PNG, JPEG, TIFF, BMP, WEBP). PDFs, Word
// files and pictures are read by the AI worker (pictures and scans with
// Tesseract); the plain formats are read here at once, with no tool to run.
// CV Studio strips RTF and ODT markup with a pattern, which leaves font
// tables, accents and joined-up paragraphs; these readers follow the
// formats instead.
package filekind

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Content types of the files accepted.
const (
	PDF  = "application/pdf"
	DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	DOC  = "application/msword"
	ODT  = "application/vnd.oasis.opendocument.text"
	RTF  = "application/rtf"
	TXT  = "text/plain"
	PNG  = "image/png"
	JPEG = "image/jpeg"
	TIFF = "image/tiff"
	BMP  = "image/bmp"
	WEBP = "image/webp"
)

// Accepted is what to tell someone whose file is not one of them.
const Accepted = "a PDF, a Word file (.docx or .doc), a text file (.txt, .rtf or .odt) or a picture of the CV (PNG, JPG, TIFF, BMP or WEBP)"

var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// Detect returns the content type of a file from its bytes. A plain text file
// has no signature, so it is taken as one only when named .txt (or .md, .text)
// and it reads as text.
func Detect(name string, data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return PDF, true
	case bytes.HasPrefix(data, oleMagic):
		// Old Word files are OLE compound documents (as are old Excel and
		// PowerPoint files, which the AI worker refuses with advice).
		return DOC, true
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return zipKind(data)
	case bytes.HasPrefix(data, []byte(`{\rtf`)):
		return RTF, true
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return PNG, true
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return JPEG, true
	case bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")):
		return TIFF, true
	case isBMP(data):
		return BMP, true
	case len(data) > 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return WEBP, true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".text", ".md":
		if _, err := Text(TXT, data); err == nil {
			return TXT, true
		}
	}
	return "", false
}

// isBMP checks more than "BM" (which many text files start with): the size
// of the bitmap's header must be one BMP files use.
func isBMP(data []byte) bool {
	if len(data) < 26 || !bytes.HasPrefix(data, []byte("BM")) {
		return false
	}
	switch binary.LittleEndian.Uint32(data[14:18]) {
	case 12, 40, 52, 56, 64, 108, 124:
		return true
	}
	return false
}

func zipKind(data []byte) (string, bool) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false
	}
	for _, f := range r.File {
		switch f.Name {
		case "word/document.xml":
			return DOCX, true
		case "mimetype":
			if m, err := readEntry(f, 200); err == nil && strings.TrimSpace(string(m)) == ODT {
				return ODT, true
			}
		}
	}
	return "", false
}

// IsImage reports a picture (read with OCR by the AI worker).
func IsImage(contentType string) bool { return strings.HasPrefix(contentType, "image/") }

// ReadHere reports a format whose text is read here, at once.
func ReadHere(contentType string) bool {
	return contentType == TXT || contentType == RTF || contentType == ODT
}

// Office reports a document LibreOffice can turn into a PDF to preview.
func Office(contentType string) bool {
	return contentType == DOCX || contentType == DOC || contentType == RTF || contentType == ODT
}

// Short is the file's type as people say it ("PDF", "Word", "Picture").
func Short(contentType string) string {
	switch {
	case contentType == PDF:
		return "PDF"
	case contentType == DOCX || contentType == DOC:
		return "Word"
	case contentType == ODT:
		return "ODT"
	case contentType == RTF:
		return "RTF"
	case contentType == TXT:
		return "Text"
	case IsImage(contentType):
		return "Picture"
	}
	return "File"
}

// clean keeps text PostgreSQL can store, with tidy line ends and no runs of
// blank lines.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\u00a0")
		if strings.TrimSpace(l) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func validText(s string) bool {
	if s == "" {
		return false
	}
	bad := 0
	for _, r := range s {
		if r == utf8.RuneError || (r < 0x20 && r != '\n' && r != '\t' && r != '\r' && r != '\f') {
			bad++
		}
	}
	return bad*20 < utf8.RuneCountInString(s) // under 5% unreadable
}
