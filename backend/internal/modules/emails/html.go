package emails

// Formatted messages: what HTML a message may hold (what the composer's
// toolbar makes, nothing that runs or loads from elsewhere), its plain-text
// version for mail apps that show no HTML, and the banners it shows.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var bannerIDRe = regexp.MustCompile(`^ebn_[0-9a-hjkmnp-tv-z]{26}$`)

// policy keeps text formatting, lists, alignment, links and banner images
// (by their id: an image's address is never kept, so a message loads nothing
// from elsewhere).
var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "div", "span", "b", "strong", "i", "em", "u", "s", "strike", "del",
		"ul", "ol", "li", "blockquote", "h1", "h2", "h3", "h4", "hr", "font", "sub", "sup",
		"table", "thead", "tbody", "tr", "td", "th")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("data-banner-id").Matching(bannerIDRe).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("align").OnElements("p", "div", "td", "th")
	p.AllowStyles("color", "background-color", "font-family", "font-size", "font-weight", "font-style",
		"text-decoration", "text-align", "margin-left", "padding-left", "line-height").Globally()
	p.AllowStyles("width", "max-width", "height").OnElements("img")
	return p
}()

// SanitizeHTML keeps only what a message may hold; an image without a
// banner id is dropped.
func SanitizeHTML(s string) string {
	clean := policy.Sanitize(s)
	return strings.TrimSpace(imgWithoutBanner.ReplaceAllString(clean, ""))
}

var imgWithoutBanner = regexp.MustCompile(`<img(?:\s+(?:alt|style)="[^"]*")*\s*/?>`)

// BannerIDs are the banners a message shows, once each.
func BannerIDs(s string) []string {
	var out []string
	for _, m := range bannerAttrRe.FindAllStringSubmatch(s, -1) {
		if !contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

var bannerAttrRe = regexp.MustCompile(`data-banner-id="(ebn_[0-9a-hjkmnp-tv-z]{26})"`)

// WithBannerCIDs points each banner image at its part in the email (cid:).
func WithBannerCIDs(s string) string {
	return bannerAttrRe.ReplaceAllString(s, `src="cid:$1" data-banner-id="$1"`)
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// TextFromHTML is the plain-text version of a message: paragraphs and line
// breaks kept, list items as "- ", links with their address.
func TextFromHTML(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node, ordered bool, index *int)
	walk = func(n *html.Node, ordered bool, index *int) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(spaces.ReplaceAllString(n.Data, " "))
			return
		case html.ElementNode:
			switch n.Data {
			case "br":
				b.WriteString("\n")
				return
			case "img":
				return
			case "li":
				*index++
				if ordered {
					b.WriteString("\n" + itoa(*index) + ". ")
				} else {
					b.WriteString("\n- ")
				}
			case "p", "div", "h1", "h2", "h3", "h4", "blockquote", "ul", "ol", "table", "tr", "hr":
				b.WriteString("\n")
			}
		case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
		}
		i := 0
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, n.Data == "ol" || (ordered && n.Data != "ul"), &i)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "h1", "h2", "h3", "h4", "blockquote", "ul", "ol", "table":
				b.WriteString("\n")
			case "td", "th":
				b.WriteString("  ")
			case "a":
				if href := attr(n, "href"); href != "" && !strings.Contains(b.String(), href) {
					b.WriteString(" (" + strings.TrimPrefix(href, "mailto:") + ")")
				}
			}
		}
	}
	i := 0
	walk(doc, false, &i)
	lines := strings.Split(b.String(), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

var spaces = regexp.MustCompile(`\s+`)

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }
