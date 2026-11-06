package cvformats

// Filling Word (.docx) templates, as pure functions on bytes.
//
// A template is an ordinary .docx with placeholders in its text:
//   - {{name}} is replaced with a value. A paragraph whose placeholders all
//     have no value is removed, so "Email: {{candidate.email}}" disappears
//     with a hidden email.
//   - {{#list}} ... {{/list}}, each marker alone in its paragraph, repeats the
//     paragraphs between them for each item of a list (experience,
//     education); inside, the item's fields ({{title}}, ...) fill in.
//   - {{#name?}} ... {{/name?}} keeps the paragraphs between once when name (a
//     value or a list) is not empty, and removes them otherwise: for headings
//     like "Experience".
//
// Word often splits typed text over several runs ("{{cand", "idate}}"): the
// text of a paragraph holding a placeholder is joined into its first run,
// which keeps that run's formatting.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// Data is what fills a template.
type Data struct {
	Values map[string]string
	Lists  map[string][]map[string]string
}

var (
	placeholderRe = regexp.MustCompile(`\{\{\s*([#/]?)([A-Za-z][A-Za-z0-9_.]*\??)\s*\}\}`)
	textRe        = regexp.MustCompile(`(?s)(<w:t(?:\s[^>]*)?>)(.*?)(</w:t>)`)
	markerRe      = regexp.MustCompile(`^\{\{\s*([#/])([A-Za-z][A-Za-z0-9_.]*\??)\s*\}\}$`)
)

// templateParts are the parts of a .docx that hold text with placeholders.
func templatePart(name string) bool {
	return name == "word/document.xml" || strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer")
}

const maxPart = 20 << 20

// readParts opens a .docx and returns its files in order.
func readParts(docx []byte) ([]*zip.File, map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, nil, errors.New("not a Word (.docx) file")
	}
	contents := map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		total += int64(f.UncompressedSize64) //nolint:gosec // sizes of a few files
		if total > 10*maxPart {
			return nil, nil, errors.New("the file unpacks to too much data")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxPart+1))
		_ = rc.Close()
		if err != nil {
			return nil, nil, err
		}
		if len(b) > maxPart {
			return nil, nil, errors.New("the file has a part that is too large")
		}
		contents[f.Name] = b
	}
	if _, ok := contents["word/document.xml"]; !ok {
		return nil, nil, errors.New("not a Word (.docx) file")
	}
	return zr.File, contents, nil
}

// Render fills a template.
func Render(template []byte, data Data) ([]byte, error) {
	files, contents, err := readParts(template)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range files {
		b := contents[f.Name]
		if templatePart(f.Name) {
			filled, err := fillPart(string(b), data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			b = []byte(filled)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Found is what a template uses: placeholders outside blocks, and for each
// block the names used inside it.
type Found struct {
	Placeholders []string
	Blocks       map[string][]string
}

// Inspect reads a template's placeholders and blocks, checking that blocks
// open and close.
func Inspect(template []byte) (Found, error) {
	found := Found{Blocks: map[string][]string{}}
	_, contents, err := readParts(template)
	if err != nil {
		return found, err
	}
	for name, b := range contents {
		if !templatePart(name) {
			continue
		}
		var open string
		for _, seg := range segments(string(b)) {
			if !seg.para {
				continue
			}
			text := paragraphText(seg.xml)
			if m := markerRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil {
				switch {
				case m[1] == "#" && open != "":
					return found, fmt.Errorf("{{#%s}} starts inside {{#%s}}: blocks cannot be nested", m[2], open)
				case m[1] == "#":
					open = m[2]
					found.Blocks[open] = append(found.Blocks[open], []string{}...)
				case m[2] != open:
					return found, fmt.Errorf("{{/%s}} does not close an open block", m[2])
				default:
					open = ""
				}
				continue
			}
			for _, pm := range placeholderRe.FindAllStringSubmatch(text, -1) {
				if pm[1] != "" {
					return found, fmt.Errorf("{{%s%s}} must be alone in its paragraph", pm[1], pm[2])
				}
				if open != "" {
					found.Blocks[open] = append(found.Blocks[open], pm[2])
				} else {
					found.Placeholders = append(found.Placeholders, pm[2])
				}
			}
		}
		if open != "" {
			return found, fmt.Errorf("{{#%s}} is not closed with {{/%s}}", open, open)
		}
	}
	return found, nil
}

// segment is a paragraph, or the XML between paragraphs.
type segment struct {
	xml  string
	para bool
}

// segments splits a part into paragraphs (<w:p>...</w:p>, with any nested
// ones, e.g. in text boxes, kept inside) and the XML between them.
func segments(xml string) []segment {
	var out []segment
	rest := xml
	for {
		start := nextParagraph(rest, 0)
		if start < 0 {
			out = append(out, segment{xml: rest})
			return out
		}
		end := paragraphEnd(rest, start)
		if end < 0 {
			out = append(out, segment{xml: rest})
			return out
		}
		out = append(out, segment{xml: rest[:start]}, segment{xml: rest[start:end], para: true})
		rest = rest[end:]
	}
}

// nextParagraph finds the next opening <w:p> or <w:p ...> (not <w:pPr>, and
// not an empty <w:p/>) from i.
func nextParagraph(s string, i int) int {
	for {
		j := strings.Index(s[i:], "<w:p")
		if j < 0 {
			return -1
		}
		j += i
		k := j + len("<w:p")
		if k < len(s) && (s[k] == '>' || s[k] == ' ') {
			close := strings.IndexByte(s[k:], '>')
			if close > 0 && s[k+close-1] == '/' {
				i = k // <w:p ... /> holds no text
				continue
			}
			return j
		}
		i = k
	}
}

// paragraphEnd returns where the paragraph opening at start ends, counting
// nested paragraphs.
func paragraphEnd(s string, start int) int {
	depth, i := 0, start
	for {
		next := nextParagraph(s, i)
		end := strings.Index(s[i:], "</w:p>")
		if end < 0 {
			return -1
		}
		end += i
		if next >= 0 && next < end {
			depth++
			i = next + len("<w:p")
			continue
		}
		depth--
		i = end + len("</w:p>")
		if depth == 0 {
			return i
		}
	}
}

func paragraphText(p string) string {
	var b strings.Builder
	for _, m := range textRe.FindAllStringSubmatch(p, -1) {
		b.WriteString(html.UnescapeString(m[2]))
	}
	return b.String()
}

// joinRuns puts a paragraph's whole text into its first text element.
func joinRuns(p string) string {
	text := paragraphText(p)
	first := true
	return textRe.ReplaceAllStringFunc(p, func(m string) string {
		if !first {
			return textRe.ReplaceAllString(m, "${1}${3}")
		}
		first = false
		return `<w:t xml:space="preserve">` + escape(text) + `</w:t>`
	})
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// fill replaces a paragraph's placeholders; ok is false when it held some
// and all were empty (the paragraph goes).
//
// A paragraph holding a text box (a drawing anchored in it) has the box's
// paragraphs inside it: each of those is filled on its own, and kept even
// when empty (a text box needs its paragraphs), so the box and its other
// text (a label such as "ABOUT HIM / HER") stay as they are.
func fill(p string, lookup func(string) string) (string, bool) {
	text := paragraphText(p)
	if !strings.Contains(text, "{{") {
		return p, true
	}
	found, filled := false, false
	if open := strings.IndexByte(p, '>') + 1; open > 0 && strings.HasSuffix(p, "</w:p>") && nextParagraph(p[open:len(p)-len("</w:p>")], 0) >= 0 {
		var b strings.Builder
		b.WriteString(p[:open])
		for _, s := range segments(p[open : len(p)-len("</w:p>")]) {
			if s.para {
				inner, _ := fill(s.xml, lookup)
				b.WriteString(inner)
				continue
			}
			b.WriteString(replaceTexts(s.xml, lookup, &found, &filled)) // the paragraph's own runs, not joined
		}
		b.WriteString("</w:p>")
		return b.String(), !found || filled
	}
	p = replaceTexts(joinRuns(p), lookup, &found, &filled)
	return p, !found || filled
}

// replaceTexts fills the placeholders in each text element of xml.
func replaceTexts(xml string, lookup func(string) string, found, filled *bool) string {
	return textRe.ReplaceAllStringFunc(xml, func(m string) string {
		parts := textRe.FindStringSubmatch(m)
		body := placeholderRe.ReplaceAllStringFunc(html.UnescapeString(parts[2]), func(ph string) string {
			*found = true
			v := lookup(placeholderRe.FindStringSubmatch(ph)[2])
			if strings.TrimSpace(v) != "" {
				*filled = true
			}
			return v
		})
		// Escape, then turn the values' line breaks into Word line breaks.
		out := strings.ReplaceAll(escape(body), "\n", `</w:t><w:br/><w:t xml:space="preserve">`)
		return parts[1] + out + parts[3]
	})
}

// fillPart fills one part: blocks first, then every paragraph.
func fillPart(xml string, data Data) (string, error) {
	segs := segments(xml)
	var out strings.Builder
	global := func(name string) string { return data.Values[name] }
	emit := func(seg segment, lookup func(string) string, prev, next string) {
		if !seg.para {
			out.WriteString(seg.xml)
			return
		}
		filled, keep := fill(seg.xml, lookup)
		switch {
		case keep:
			out.WriteString(filled)
		case onlyInCell(prev, next):
			out.WriteString("<w:p/>") // a table cell needs a paragraph
		}
	}
	for i := 0; i < len(segs); i++ {
		seg := segs[i]
		m := markerRe.FindStringSubmatch(strings.TrimSpace(paragraphText(seg.xml)))
		if !seg.para || m == nil || m[1] != "#" {
			emit(seg, global, before(segs, i), after(segs, i))
			continue
		}
		name := m[2]
		end := -1
		for j := i + 1; j < len(segs); j++ {
			if c := markerRe.FindStringSubmatch(strings.TrimSpace(paragraphText(segs[j].xml))); segs[j].para && c != nil && c[1] == "/" && c[2] == name {
				end = j
				break
			}
		}
		if end < 0 {
			return "", fmt.Errorf("{{#%s}} is not closed with {{/%s}}", name, name)
		}
		body := segs[i+1 : end]
		for _, item := range items(name, data) {
			lookup := func(key string) string {
				if v, ok := item[key]; ok {
					return v
				}
				return data.Values[key]
			}
			for k, s := range body {
				emit(s, lookup, before(body, k), after(body, k))
			}
		}
		i = end
	}
	return out.String(), nil
}

// items are what a block repeats for: a list's items; or, for name?, once
// when the value or list is not empty.
func items(name string, data Data) []map[string]string {
	if cond, ok := strings.CutSuffix(name, "?"); ok {
		if len(data.Lists[cond]) > 0 || strings.TrimSpace(data.Values[cond]) != "" {
			return []map[string]string{{}}
		}
		return nil
	}
	return data.Lists[name]
}

func before(segs []segment, i int) string {
	if i > 0 {
		return segs[i-1].xml
	}
	return ""
}

func after(segs []segment, i int) string {
	if i+1 < len(segs) {
		return segs[i+1].xml
	}
	return ""
}

// onlyInCell reports whether a paragraph is alone in a table cell.
func onlyInCell(prev, next string) bool {
	p, n := strings.TrimSpace(prev), strings.TrimSpace(next)
	return strings.HasPrefix(n, "</w:tc>") && (strings.HasSuffix(p, "</w:tcPr>") || strings.HasSuffix(p, "<w:tcPr/>") || strings.HasSuffix(p, "<w:tc>"))
}
