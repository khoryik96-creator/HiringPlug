package industries

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Key is comparison-only; the user's display label is never rewritten.
func Key(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= '\u2010' && r <= '\u2015' || r == '\u2212' {
			return '-'
		}
		return unicode.ToLower(r)
	}, norm.NFC.String(s))
	s = strings.Join(strings.Fields(s), " ")
	parts := strings.Split(s, "-")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "-")
}

func invalid(field, message string) error {
	return apperr.Validation(apperr.FieldErrors{{Field: field, Message: message}})
}
