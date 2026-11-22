// Package sheet writes tables as files people open in Excel or Google
// Sheets: CSV (UTF-8, with the byte-order mark Excel needs for accents) and
// .xlsx (one sheet per table, the heading row bold and frozen). It needs no
// library: an .xlsx is a zip of a few XML files.
package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Formats.
const (
	CSV  = "csv"
	XLSX = "xlsx"
)

// MaxRows is the most rows an export holds (a search that finds more must
// be narrowed).
const MaxRows = 10000

// Kind is how a column's cells are written in .xlsx.
type Kind int

// Column kinds.
const (
	Text   Kind = iota
	Number      // a plain number ("12500.00"): Excel can add it up
)

// Column is a heading and how its cells are written.
type Column struct {
	Heading string
	Kind    Kind
}

// Table is one sheet: its name (at most 31 characters in Excel), its columns
// and its rows (one cell per column; "" is empty).
type Table struct {
	Name    string
	Columns []Column
	Rows    [][]string
}

// File is a written file.
type File struct {
	Data        []byte
	ContentType string
	Name        string // the file name, with the extension
}

// Write writes the tables in a format (csv or xlsx) as name.<ext>. A CSV
// holds one table per section, each under its name when there are several.
func Write(format, name string, tables ...Table) (File, error) {
	switch format {
	case CSV:
		data, err := writeCSV(tables)
		return File{Data: data, ContentType: "text/csv; charset=utf-8", Name: name + ".csv"}, err
	case XLSX:
		data, err := writeXLSX(tables)
		return File{Data: data, ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Name: name + ".xlsx"}, err
	}
	return File{}, fmt.Errorf("sheet: unknown format %q", format)
}

// SafeCell keeps a spreadsheet from reading a cell as a formula (=, +, -,
// @ at the start can run commands when a CSV is opened): such text gets a
// leading apostrophe, which Excel shows as plain text.
func SafeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return s // a negative number is a number
		}
		return "'" + s
	}
	return s
}

func writeCSV(tables []Table) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\uFEFF")
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	for i, t := range tables {
		if len(tables) > 1 {
			if i > 0 {
				_ = w.Write([]string{})
			}
			_ = w.Write([]string{SafeCell(t.Name)})
		}
		head := make([]string, len(t.Columns))
		for j, c := range t.Columns {
			head[j] = SafeCell(c.Heading)
		}
		if err := w.Write(head); err != nil {
			return nil, err
		}
		for _, r := range t.Rows {
			row := make([]string, len(r))
			for j, c := range r {
				row[j] = SafeCell(c)
			}
			if err := w.Write(row); err != nil {
				return nil, err
			}
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// sheetName makes a valid, unique Excel sheet name.
func sheetName(name string, used map[string]bool) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" {
		name = "Sheet"
	}
	if r := []rune(name); len(r) > 31 {
		name = string(r[:31])
	}
	base := name
	for n := 2; used[strings.ToLower(name)]; n++ {
		suffix := fmt.Sprintf(" (%d)", n)
		r := []rune(base)
		if len(r)+len(suffix) > 31 {
			r = r[:31-len(suffix)]
		}
		name = string(r) + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}

func writeXLSX(tables []Table) ([]byte, error) {
	if len(tables) == 0 {
		tables = []Table{{Name: "Sheet"}}
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	add := func(path, body string) error {
		f, err := z.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Deflate, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		_, err = f.Write([]byte(body))
		return err
	}
	var types, sheets, rels strings.Builder
	used := map[string]bool{}
	for i, t := range tables {
		n := i + 1
		fmt.Fprintf(&types, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, n)
		fmt.Fprintf(&sheets, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escape(sheetName(t.Name, used)), n, n)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, n, n)
	}
	n := len(tables)
	files := []struct{ path, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			types.String() + `</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets>` + sheets.String() + `</sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() +
			fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, n+1) +
			`</Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
			`</styleSheet>`},
	}
	for _, f := range files {
		if err := add(f.path, f.body); err != nil {
			return nil, err
		}
	}
	for i, t := range tables {
		if err := add(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), worksheet(t)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func worksheet(t Table) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	if len(t.Columns) > 0 {
		b.WriteString("<cols>")
		for j, c := range t.Columns {
			width := 12
			if c.Kind == Text {
				width = 22
			}
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, j+1, j+1, width)
		}
		b.WriteString("</cols>")
	}
	b.WriteString("<sheetData>")
	b.WriteString(`<row r="1">`)
	for j, c := range t.Columns {
		fmt.Fprintf(&b, `<c r="%s1" t="inlineStr" s="1"><is><t xml:space="preserve">%s</t></is></c>`, colName(j), escape(c.Heading))
	}
	b.WriteString("</row>")
	for i, r := range t.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, i+2)
		for j, v := range r {
			if v == "" {
				continue
			}
			ref := colName(j) + strconv.Itoa(i+2)
			if j < len(t.Columns) && t.Columns[j].Kind == Number {
				if _, err := strconv.ParseFloat(v, 64); err == nil {
					fmt.Fprintf(&b, `<c r="%s"><v>%s</v></c>`, ref, v)
					continue
				}
			}
			fmt.Fprintf(&b, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, escape(v))
		}
		b.WriteString("</row>")
	}
	b.WriteString("</sheetData></worksheet>")
	return b.String()
}

// colName is a column's letters: 0 is A, 25 is Z, 26 is AA.
func colName(j int) string {
	s := ""
	for j++; j > 0; j = (j - 1) / 26 {
		s = string(rune('A'+(j-1)%26)) + s
	}
	return s
}

// escape writes text for XML, leaving out characters XML 1.0 cannot hold.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != 0xFFFE && r != 0xFFFF):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Collect pages through a list: next returns a page's rows and the cursor
// of the next page (nil after the last). More than MaxRows is
// EXPORT_TOO_LARGE.
func Collect[T any](next func(cursor *string) ([]T, *string, error)) ([]T, error) {
	var out []T
	var cursor *string
	for {
		rows, more, err := next(cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(out) > MaxRows {
			return nil, apperr.ExportTooLarge(MaxRows)
		}
		if more == nil {
			return out, nil
		}
		cursor = more
	}
}

// Str is a value or "".
func Str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Day writes a date as 2026-11-22 (Excel and Sheets read it as a date).
func Day(t time.Time) string { return t.Format("2006-01-02") }

// Dated is a file name with today's date: "Companies 22 Nov 2026".
func Dated(what string, now time.Time) string { return what + " " + now.Format("2 Jan 2006") }
