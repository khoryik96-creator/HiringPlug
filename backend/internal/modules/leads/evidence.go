package leads

import (
	"context"
	"html"
	"strings"

	htmltoken "golang.org/x/net/html"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
)

// Descriptions are displayed as text. Keep paragraph boundaries, skip
// script/style bodies and bound what is retained from upstream.
func postingDescription(raw string, markup bool) *string {
	if markup {
		raw = html.UnescapeString(raw)
		t := htmltoken.NewTokenizer(strings.NewReader(raw))
		var b strings.Builder
		skip := false
	readDescription:
		for b.Len() < 64_000 {
			token := t.Next()
			switch token {
			case htmltoken.ErrorToken:
				break readDescription
			case htmltoken.CommentToken, htmltoken.DoctypeToken:
			case htmltoken.StartTagToken, htmltoken.SelfClosingTagToken, htmltoken.EndTagToken:
				tag, _ := t.TagName()
				switch string(tag) {
				case "script", "style":
					skip = token == htmltoken.StartTagToken
				case "p", "li", "div", "br", "h1", "h2", "h3", "h4":
					if !skip {
						b.WriteByte('\n')
					}
				}
			case htmltoken.TextToken:
				if !skip {
					b.Write(t.Text())
				}
			}
		}
		raw = b.String()
	}
	lines := []string{}
	for line := range strings.SplitSeq(raw, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	text := []rune(strings.Join(lines, "\n"))
	return str(string(text[:min(16_000, len(text))]))
}

func recordDescription(ctx context.Context, q *store.Queries, id string, description *string) error {
	if description == nil {
		return nil // keep the previous evidence when the source omits the full ad
	}
	return q.SaveLeadEvidence(ctx, store.SaveLeadEvidenceParams{LeadID: id, Description: *description})
}
