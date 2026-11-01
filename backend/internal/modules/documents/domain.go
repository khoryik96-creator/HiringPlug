package documents

import (
	"archive/zip"
	"bytes"
	"strings"
	"time"
	"unicode"
)

// Supported file types.
const (
	ContentTypePDF  = "application/pdf"
	ContentTypeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// Document statuses (mirroring the contract's DocumentStatus enum).
const (
	StatusQueued    = "queued"
	StatusExtracted = "extracted"
	StatusNeedsOCR  = "needs_ocr"
	StatusFailed    = "failed"
)

// Document is a file attached to a candidate.
type Document struct {
	ID            string
	CandidateID   string
	Source        string
	FileName      string
	ContentType   string
	SizeBytes     int64
	SHA256        string
	Status        string
	PageCount     *int
	Extractor     *string
	ErrorMessage  *string
	Warnings      []string // e.g. pages OCR could not read
	ExtractedText *string  // only loaded for single-document reads
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DetectContentType identifies a PDF or a Word (.docx) file from its bytes,
// never trusting the name or the browser's claimed type alone.
func DetectContentType(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return ContentTypePDF, true
	case bytes.HasPrefix(data, []byte("PK\x03\x04")) && isDOCX(data):
		return ContentTypeDOCX, true
	default:
		return "", false
	}
}

func isDOCX(data []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			return true
		}
	}
	return false
}

// CleanFileName keeps just the base name, without control characters or
// path separators, at most 255 characters.
func CleanFileName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if runes := []rune(name); len(runes) > 255 {
		name = string(runes[:255])
	}
	if name == "" || name == "." || name == ".." {
		return "upload"
	}
	return name
}

// CleanExtractedText removes characters PostgreSQL cannot store in text.
func CleanExtractedText(s string) string {
	return strings.ReplaceAll(s, "\x00", "")
}
