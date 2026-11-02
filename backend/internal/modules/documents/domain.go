package documents

import (
	"archive/zip"
	"bytes"
	"regexp"
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
	ParseStatus   *string  // queued, parsed, failed; nil when never parsed
	ParsedProfile []byte   // JSON (api.ParsedProfile); only loaded for single-document reads
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

// These rules decide which parsed CV values are kept. They hold for any
// reader (rules today, an AI model later): a value is kept only if its quote
// is text from the CV and the value can be read from that quote.

// Quotes reports whether quote appears in the CV text, ignoring case and
// differences in spacing. Prepare the text once with ForQuotes.
func Quotes(preparedText, quote string) bool {
	q := ForQuotes(quote)
	return q != "" && strings.Contains(preparedText, q)
}

// ForQuotes lower-cases s and collapses whitespace, for Quotes.
func ForQuotes(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// squash keeps only letters and digits, lower-cased: "+60 12-345" -> "6012345".
// "]" and "!" count as "i": OCR often reads a final I that way ("BINT]"), and
// the parser reads it back as a letter.
func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ']' || r == '!':
			b.WriteRune('i')
		}
	}
	return b.String()
}

var (
	urlScheme    = regexp.MustCompile(`(?i)^https?://(www\.)?`)
	countryCode  = regexp.MustCompile(`^[A-Z]{2}$`)
	noticePeriod = regexp.MustCompile(`^\d{1,3} (day|week|month)$`)
)

// Field kinds with their own value rules.
const (
	FieldText         = ""
	FieldURL          = "url"
	FieldCountry      = "country"
	FieldNoticePeriod = "notice"
)

// ValueFromQuote reports whether value can be read from quote: the same
// letters and digits in order (a tidied "Siti Aisyah" from "SITI AISYAH",
// "Excel" from "Excel (Advanced)"), or a fixed format for coded fields.
func ValueFromQuote(kind, value, quote string) bool {
	switch kind {
	case FieldCountry:
		return countryCode.MatchString(value)
	case FieldNoticePeriod:
		return noticePeriod.MatchString(value)
	case FieldURL:
		value = urlScheme.ReplaceAllString(value, "")
	}
	v := squash(value)
	return v != "" && strings.Contains(squash(quote), v)
}
