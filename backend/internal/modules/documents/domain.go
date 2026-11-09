package documents

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
)

// Supported file types (filekind has them all).
const (
	ContentTypePDF  = filekind.PDF
	ContentTypeDOCX = filekind.DOCX
	ContentTypeDOC  = filekind.DOC
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
	PdfStatus     *string  // formatted CVs made by the app: queued, ready, failed
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DetectContentType identifies an uploaded file from its bytes (and, for a
// plain text file, its name): see filekind.
func DetectContentType(name string, data []byte) (string, bool) {
	return filekind.Detect(name, data)
}

// Readable reports a file of a type taken whose text can be read (a text,
// RTF or ODT file is read at once; the others by the AI worker).
func Readable(name string, data []byte) bool {
	ct, ok := DetectContentType(name, data)
	if ok && filekind.ReadHere(ct) {
		_, err := filekind.Text(ct, data)
		return err == nil
	}
	return ok
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
// is text from the CV (platform/quote) and the value can be read from that
// quote.

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
	// FieldInferred is a value worked out from the quote (an employer's
	// industry from its name): only the quote must be in the CV.
	FieldInferred = "inferred"
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
	case FieldInferred:
		n := utf8.RuneCountInString(strings.TrimSpace(value))
		return n > 0 && n <= 100
	case FieldURL:
		value = urlScheme.ReplaceAllString(value, "")
	}
	v := squash(value)
	return v != "" && strings.Contains(squash(quote), v)
}
