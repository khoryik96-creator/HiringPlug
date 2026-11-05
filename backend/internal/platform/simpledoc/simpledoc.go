// Package simpledoc writes plain documents (a title, a muted line, headings,
// paragraphs and bullet points) as Word (.docx) and PDF files, for exports
// such as job ads. Text only: nothing in it is markup.
package simpledoc

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Kind is what a block is.
type Kind int

// The kinds of block.
const (
	Title Kind = iota
	Muted
	Heading
	Paragraph
	Bullet
)

// Block is one part of a document.
type Block struct {
	Kind Kind
	Text string
}

// Doc is a document: blocks in order.
type Doc []Block

// FromText reads plain text as paragraphs and bullet points (lines starting
// "- ", "• " or "* "), the way the app shows an ad's text.
func FromText(text string) Doc {
	var out Doc
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: Paragraph, Text: strings.Join(para, "\n")})
			para = nil
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "• ") || strings.HasPrefix(line, "* "):
			flush()
			_, rest, _ := strings.Cut(line, " ")
			out = append(out, Block{Kind: Bullet, Text: strings.TrimSpace(rest)})
		default:
			para = append(para, line)
		}
	}
	flush()
	return out
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func run(text string, props string) string {
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString(`<w:r><w:br/></w:r>`)
		}
		fmt.Fprintf(&b, `<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, props, esc(line))
	}
	return b.String()
}

// Docx returns the document as a Word file.
func (d Doc) Docx() ([]byte, error) {
	var body strings.Builder
	for _, b := range d {
		switch b.Kind {
		case Title:
			body.WriteString(`<w:p><w:pPr><w:spacing w:after="120"/></w:pPr>` + run(b.Text, `<w:rPr><w:b/><w:sz w:val="36"/></w:rPr>`) + `</w:p>`)
		case Muted:
			body.WriteString(`<w:p><w:pPr><w:spacing w:after="240"/></w:pPr>` + run(b.Text, `<w:rPr><w:color w:val="6B7280"/><w:sz w:val="20"/></w:rPr>`) + `</w:p>`)
		case Heading:
			body.WriteString(`<w:p><w:pPr><w:spacing w:before="240" w:after="80"/></w:pPr>` + run(b.Text, `<w:rPr><w:b/><w:sz w:val="26"/></w:rPr>`) + `</w:p>`)
		case Bullet:
			body.WriteString(`<w:p><w:pPr><w:ind w:left="360" w:hanging="360"/><w:spacing w:after="60"/></w:pPr>` + run("•\t"+b.Text, "") + `</w:p>`)
		default:
			body.WriteString(`<w:p><w:pPr><w:spacing w:after="160"/></w:pPr>` + run(b.Text, "") + `</w:p>`)
		}
	}
	files := []struct{ name, data string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
			body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134" w:header="0" w:footer="0" w:gutter="0"/></w:sectPr></w:body></w:document>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.data)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PDF returns the document as an A4 PDF. It uses the standard Helvetica
// font, which covers English and Malay; characters beyond Western European
// ones (e.g. Chinese) print as "?".
func (d Doc) PDF() ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("") // UTF-8 to the font's code page
	for _, b := range d {
		switch b.Kind {
		case Title:
			pdf.SetFont("Helvetica", "B", 18)
			pdf.SetTextColor(17, 24, 39)
			pdf.MultiCell(0, 8, tr(b.Text), "", "L", false)
			pdf.Ln(2)
		case Muted:
			pdf.SetFont("Helvetica", "", 10)
			pdf.SetTextColor(107, 114, 128)
			pdf.MultiCell(0, 5, tr(b.Text), "", "L", false)
			pdf.Ln(5)
		case Heading:
			pdf.SetFont("Helvetica", "B", 12)
			pdf.SetTextColor(17, 24, 39)
			pdf.Ln(2)
			pdf.MultiCell(0, 6, tr(b.Text), "", "L", false)
		case Bullet:
			pdf.SetFont("Helvetica", "", 11)
			pdf.SetTextColor(31, 41, 55)
			x := pdf.GetX()
			pdf.CellFormat(6, 5.5, tr("•"), "", 0, "L", false, 0, "")
			pdf.MultiCell(0, 5.5, tr(b.Text), "", "L", false)
			pdf.SetX(x)
			pdf.Ln(1)
		default:
			pdf.SetFont("Helvetica", "", 11)
			pdf.SetTextColor(31, 41, 55)
			pdf.MultiCell(0, 5.5, tr(b.Text), "", "L", false)
			pdf.Ln(3)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
