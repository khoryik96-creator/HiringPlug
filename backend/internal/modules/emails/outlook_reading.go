package emails

// Reading a received message as Outlook shows it: its formatting (tables,
// fonts, colours, the sender's signature and logo) made safe to show. The
// web app puts it in a sandboxed frame with no scripts, so its styles stay
// inside the message. Nothing runs and nothing loads from elsewhere until
// the web app shows it: pictures sent with the message (cid:) are included,
// pictures from the web are marked (data-hp-src) and counted, for the web
// app to load (or, in Junk Email, to hold back until asked).

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	readingHTMLLimit   = 2 << 20 // a message's HTML shown at most (bigger: plain text)
	readingStyleLimit  = 64 << 10
	inlineImageLimit   = 3 << 20  // one picture sent with a message
	inlineImagesLimit  = 10 << 20 // all of them
	inlineImagesAtMost = 30
)

var readingPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("font", "center", "span", "div", "u", "s", "strike", "small", "big", "caption", "col", "colgroup")
	p.AllowAttrs("class").Globally()
	p.AllowAttrs("align", "valign", "width", "height", "bgcolor", "border", "cellpadding", "cellspacing", "colspan", "rowspan", "nowrap").
		OnElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "col", "colgroup", "caption")
	p.AllowAttrs("align").OnElements("p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "img", "center")
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("width", "height", "alt", "title", "align", "border").OnElements("img")
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid")
	p.AllowDataURIImages()
	p.RequireNoFollowOnLinks(false)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	// Styles that shape the message; none can load anything (no backgrounds
	// with pictures, no fonts from the web).
	p.AllowStyles("color", "background-color", "font", "font-family", "font-size", "font-weight", "font-style", "font-variant",
		"text-decoration", "text-align", "text-indent", "text-transform", "line-height", "letter-spacing", "word-spacing",
		"white-space", "word-break", "overflow-wrap", "vertical-align", "direction", "display", "visibility",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"border", "border-top", "border-right", "border-bottom", "border-left", "border-color", "border-style", "border-width",
		"border-radius", "border-collapse", "border-spacing", "table-layout",
		"width", "min-width", "max-width", "height", "min-height", "max-height",
		"list-style-type", "list-style-position", "float", "clear").Globally()
	return p
}()

// cssUnsafe are what a message's <style> may not hold: anything that loads
// from elsewhere, runs, or closes the style element.
var (
	cssComments = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssImport   = regexp.MustCompile(`(?i)@(import|font-face|namespace)[^;{]*(;|\{[^}]*\})`)
	cssURL      = regexp.MustCompile(`(?i)url\s*\([^)]*\)`)
	cssRunning  = regexp.MustCompile(`(?i)expression\s*\(|behaviou?r\s*:|-moz-binding|javascript:|vbscript:|@charset`)
)

func cleanCSS(css string) string {
	css = cssComments.ReplaceAllString(css, "")
	css = cssImport.ReplaceAllString(css, "")
	css = cssURL.ReplaceAllString(css, "none")
	if cssRunning.MatchString(css) || strings.ContainsAny(css, "<>\\") {
		return ""
	}
	return strings.TrimSpace(css)
}

// messageStyles are a message's <style> blocks, cleaned.
func messageStyles(doc *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "style" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					b.WriteString(cleanCSS(c.Data))
					b.WriteString("\n")
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return truncate(strings.TrimSpace(b.String()), readingStyleLimit)
}

// readableHTML is a message's HTML made safe to show: its styles, then its
// body. Pictures sent with it (cid:) come from inline (content id -> data:
// URI); pictures from the web are held back as data-hp-src and counted.
func readableHTML(raw string, inline map[string]string) (string, int) {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", 0
	}
	styles := messageStyles(doc)
	clean := readingPolicy.Sanitize(raw)
	nodes, err := html.ParseFragment(strings.NewReader(clean), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return "", 0
	}
	blocked := 0
	var fix func(n *html.Node)
	fix = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			src := attr(n, "src")
			lower := strings.ToLower(src)
			switch {
			case strings.HasPrefix(lower, "cid:"):
				setAttr(n, "src", inline[contentID(src[4:])])
			case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
				setAttr(n, "src", "")
				// Loaded only over https, as the page is (an old http address is tried over https).
				setAttr(n, "data-hp-src", "https://"+src[strings.Index(src, "://")+3:])
				blocked++
			case strings.HasPrefix(lower, "data:image/"):
			default:
				setAttr(n, "src", "")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			fix(c)
		}
	}
	var b strings.Builder
	if styles != "" {
		b.WriteString("<style>" + styles + "</style>")
	}
	for _, n := range nodes {
		fix(n)
		if err := html.Render(&b, n); err != nil {
			return "", 0
		}
	}
	return b.String(), blocked
}

// contentIDs are the pictures a message's HTML shows from its own files.
func contentIDs(raw string) map[string]bool {
	out := map[string]bool{}
	for _, m := range cidRe.FindAllStringSubmatch(raw, inlineImagesAtMost) {
		out[contentID(m[1])] = true
	}
	return out
}

var cidRe = regexp.MustCompile(`(?i)["']cid:([^"'\s]{1,300})["']`)

// contentID is a content id as Outlook files and links name it: no angle
// brackets, any case.
func contentID(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), "<>"))
}

func setAttr(n *html.Node, key, value string) {
	for i, a := range n.Attr {
		if a.Key == key {
			if value == "" && key == "src" {
				n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
				return
			}
			n.Attr[i].Val = value
			return
		}
	}
	if value != "" {
		n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
	}
}

var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/jpg": true, "image/gif": true, "image/webp": true, "image/bmp": true}
