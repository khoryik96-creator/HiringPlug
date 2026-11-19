package claims

import (
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Where a submitted receipt goes in the claims storage:
//
//	<root>/<YYYY-MM>/<person>/<receipt type>/<YYYY-MM-DD> <merchant> <CUR> <amount> (<id end>).<ext>
//
// e.g. Claims/2026-11/Aisyah Rahman/Grab fare/2026-11-03 Grab MYR 23.00 (t5v6w7).pdf.
// The month and day are the receipt's; the end of the claim's id keeps two
// receipts alike (same day, shop and amount) apart.

// Upload is what a receipt's place is made from.
type Upload struct {
	Root, Person, ReceiptType, Merchant, Currency, Amount, ClaimID, FileName, ContentType string
	ReceiptDate                                                                           time.Time
}

const segmentMax = 80 // characters in one folder or file name (the drives allow more; people read it)

// UploadPath is the receipt's folders and file name, each safe for Google
// Drive, OneDrive and Dropbox.
func UploadPath(u Upload) []string {
	out := RootFolders(u.Root)
	if len(out) == 0 {
		out = []string{"Claims"}
	}
	out = append(out, u.ReceiptDate.Format("2006-01"), segment(u.Person, "Someone"), segment(u.ReceiptType, "Other"))
	words := []string{u.ReceiptDate.Format("2006-01-02")}
	if m := segment(u.Merchant, ""); m != "" {
		words = append(words, shorten(m, 40))
	}
	words = append(words, u.Currency, u.Amount)
	if len(u.ClaimID) >= 6 {
		words = append(words, "("+u.ClaimID[len(u.ClaimID)-6:]+")")
	}
	return append(out, segment(strings.Join(words, " "), "Receipt")+extension(u.FileName, u.ContentType))
}

// RootFolders splits the configured top folder ("Finance/Claims") into its
// folders, each made safe; empty parts and "." or ".." are dropped.
func RootFolders(root string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(root, func(r rune) bool { return r == '/' || r == '\\' }) {
		if s := segment(part, ""); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ValidRoot reports whether a top folder names at least one folder, at most
// five deep.
func ValidRoot(root string) bool {
	n := len(RootFolders(root))
	return n >= 1 && n <= 5 && utf8.RuneCountInString(root) <= 200
}

// segment makes one folder or file name: no characters a drive refuses
// (\ / : * ? " < > |), no control characters, spaces squeezed, no leading or
// trailing dots or spaces (Windows and OneDrive drop them), not too long.
func segment(s, empty string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case strings.ContainsRune(`\/:*?"<>|`, r):
			b.WriteRune('-')
		case unicode.IsControl(r) || r == utf8.RuneError:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.Join(strings.Fields(b.String()), " "), ". ")
	out = shorten(out, segmentMax)
	if out == "" {
		return empty
	}
	return out
}

func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:n]), ". ")
}

// extension keeps the receipt's own (".pdf", ".jpg"), or names one from its type.
func extension(fileName, contentType string) string {
	ext := strings.ToLower(path.Ext(fileName))
	if len(ext) >= 2 && len(ext) <= 6 && strings.IndexFunc(ext[1:], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) < 0 {
		return ext
	}
	switch contentType {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/tiff":
		return ".tif"
	case "image/bmp":
		return ".bmp"
	case "image/webp":
		return ".webp"
	}
	return ""
}
