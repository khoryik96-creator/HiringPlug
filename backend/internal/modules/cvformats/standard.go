package cvformats

// The built-in "standard" CV template, built as a .docx here (so it needs no
// binary file and shows every placeholder in use). Companies download it as
// the starting point for their own.

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
	"sync"
)

// StandardID is the built-in template's id.
const StandardID = "standard"

// Standard returns the built-in template.
var Standard = sync.OnceValue(func() []byte {
	type para struct{ style, text string }
	body := []para{
		{"Title", "{{candidate.fullName}}"},
		{"Subtitle", "{{candidate.currentTitle}}"},
		{"Normal", "Position: {{job.title}}"},
		{"Normal", "Client: {{company.name}}"},
		{"Heading1", "Profile"},
		{"Normal", "Current company: {{candidate.currentCompany}}"},
		{"Normal", "Location: {{candidate.location}}"},
		{"Normal", "Nationality: {{candidate.nationality}}"},
		{"Normal", "Work authorisation: {{candidate.workAuthorization}}"},
		{"Normal", "Notice period: {{candidate.noticePeriod}}"},
		{"Normal", "Email: {{candidate.email}}"},
		{"Normal", "Phone: {{candidate.phone}}"},
		{"Normal", "Current salary: {{salary.current}}"},
		{"Normal", "Expected salary: {{salary.expected}}"},
		{"Normal", "{{#summary?}}"},
		{"Heading1", "Summary"},
		{"Normal", "{{summary}}"},
		{"Normal", "{{/summary?}}"},
		{"Normal", "{{#candidate.skills?}}"},
		{"Heading1", "Skills"},
		{"Normal", "{{candidate.skills}}"},
		{"Normal", "{{/candidate.skills?}}"},
		{"Normal", "{{#experience?}}"},
		{"Heading1", "Experience"},
		{"Normal", "{{/experience?}}"},
		{"Normal", "{{#experience}}"},
		{"Heading2", "{{title}}"},
		{"Strong", "{{company}}"},
		{"Muted", "{{dates}}"},
		{"Normal", "{{description}}"},
		{"Normal", "{{/experience}}"},
		{"Normal", "{{#education?}}"},
		{"Heading1", "Education"},
		{"Normal", "{{/education?}}"},
		{"Normal", "{{#education}}"},
		{"Heading2", "{{qualification}}"},
		{"Normal", "{{institution}}"},
		{"Muted", "{{year}}"},
		{"Normal", "{{/education}}"},
	}
	var doc strings.Builder
	doc.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`)
	for _, p := range body {
		doc.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + p.style + `"/></w:pPr><w:r><w:t xml:space="preserve">` + escape(p.text) + `</w:t></w:r></w:p>`)
	}
	doc.WriteString(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/>` +
		`<w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134" w:header="567" w:footer="567" w:gutter="0"/></w:sectPr></w:body></w:document>`)

	footer := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:pStyle w:val="Muted"/><w:jc w:val="center"/></w:pPr>` +
		`<w:r><w:t xml:space="preserve">Presented by {{agency.name}}: {{recruiter.name}}, {{recruiter.email}} · {{date}}</w:t></w:r></w:p></w:ftr>`

	style := func(id, name, based string, size int, bold bool, color string, spaceBefore int) string {
		rpr := `<w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="` + itoa(size) + `"/>`
		if bold {
			rpr += `<w:b/>`
		}
		if color != "" {
			rpr += `<w:color w:val="` + color + `"/>`
		}
		rpr += `</w:rPr>`
		based2 := ""
		if based != "" {
			based2 = `<w:basedOn w:val="` + based + `"/>`
		}
		return `<w:style w:type="paragraph" w:styleId="` + id + `"><w:name w:val="` + name + `"/>` + based2 +
			`<w:pPr><w:spacing w:before="` + itoa(spaceBefore) + `" w:after="60"/></w:pPr>` + rpr + `</w:style>`
	}
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="21"/></w:rPr></w:rPrDefault></w:docDefaults>` +
		style("Normal", "Normal", "", 21, false, "", 0) +
		style("Title", "Title", "Normal", 40, true, "1F3864", 0) +
		style("Subtitle", "Subtitle", "Normal", 26, false, "44546A", 0) +
		style("Heading1", "heading 1", "Normal", 26, true, "1F3864", 240) +
		style("Heading2", "heading 2", "Normal", 22, true, "", 160) +
		style("Strong", "Company", "Normal", 21, true, "44546A", 0) +
		style("Muted", "Muted", "Normal", 18, false, "7F7F7F", 0) +
		`</w:styles>`

	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`<Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", doc.String()},
		{"word/styles.xml", styles},
		{"word/footer1.xml", footer},
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return out.Bytes()
})

func itoa(n int) string { return strconv.Itoa(n) }
