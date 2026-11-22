package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var people = Table{
	Name:    "Candidates",
	Columns: []Column{{Heading: "Name"}, {Heading: "Expected salary", Kind: Number}, {Heading: "Notes"}},
	Rows: [][]string{
		{"Siti Nurhaliza", "12500.00", `=HYPERLINK("http://evil")`},
		{"Tan, Wei Ming", "", "-1"},
		{"Ali <&> \"quotes\"", "n/a", "line\nbreak"},
	},
}

func TestCSV(t *testing.T) {
	f, err := Write(CSV, "Candidates 22 Nov 2026", people)
	require.NoError(t, err)
	assert.Equal(t, "Candidates 22 Nov 2026.csv", f.Name)
	assert.True(t, bytes.HasPrefix(f.Data, []byte("\xEF\xBB\xBF")), "the byte-order mark Excel needs for accents")
	text := string(f.Data[3:])
	assert.Contains(t, text, "Name,Expected salary,Notes\r\n")
	assert.Contains(t, text, `"Tan, Wei Ming",,-1`, "quoted when needed; a negative number stays a number")
	assert.Contains(t, text, `"'=HYPERLINK(""http://evil"")"`, "a formula is written as text")
	assert.Contains(t, text, "\"line\r\nbreak\"", "a line break inside a cell stays in it")
}

func TestCSVWithSeveralTables(t *testing.T) {
	f, err := Write(CSV, "Report", Table{Name: "Funnel", Columns: []Column{{Heading: "Stage"}}, Rows: [][]string{{"applied"}}},
		Table{Name: "Sources", Columns: []Column{{Heading: "Source"}}, Rows: [][]string{{"jobstreet"}}})
	require.NoError(t, err)
	assert.Equal(t, "Funnel\r\nStage\r\napplied\r\n\r\nSources\r\nSource\r\njobstreet\r\n", string(f.Data[3:]))
}

type sheetXML struct {
	Rows []struct {
		Cells []struct {
			Ref    string `xml:"r,attr"`
			Type   string `xml:"t,attr"`
			Style  string `xml:"s,attr"`
			Value  string `xml:"v"`
			Inline string `xml:"is>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(r)
		require.NoError(t, err)
		out[f.Name] = string(b)
		var any struct{}
		require.NoError(t, xml.Unmarshal(b, &any), "%s is well-formed XML", f.Name)
	}
	return out
}

func TestXLSX(t *testing.T) {
	f, err := Write(XLSX, "Candidates", people, Table{Name: "Candidates", Columns: []Column{{Heading: "x"}}})
	require.NoError(t, err)
	assert.Equal(t, "Candidates.xlsx", f.Name)
	files := readZip(t, f.Data)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml"} {
		assert.Contains(t, files, name)
	}
	assert.Contains(t, files["xl/workbook.xml"], `name="Candidates"`)
	assert.Contains(t, files["xl/workbook.xml"], `name="Candidates (2)"`, "sheet names are unique")

	var s sheetXML
	require.NoError(t, xml.Unmarshal([]byte(files["xl/worksheets/sheet1.xml"]), &s))
	require.Len(t, s.Rows, 4)
	head := s.Rows[0].Cells
	assert.Equal(t, "Expected salary", head[1].Inline)
	assert.Equal(t, "1", head[1].Style, "the heading is bold")
	first := s.Rows[1].Cells
	assert.Equal(t, "B2", first[1].Ref)
	assert.Equal(t, "12500.00", first[1].Value, "a number column holds numbers")
	assert.Empty(t, first[1].Type)
	assert.Equal(t, `=HYPERLINK("http://evil")`, first[2].Inline, "text, never a formula, in .xlsx")
	assert.Len(t, s.Rows[2].Cells, 2, "empty cells are left out")
	third := s.Rows[3].Cells
	assert.Equal(t, `Ali <&> "quotes"`, third[0].Inline)
	assert.Equal(t, "inlineStr", third[1].Type, "not a number: text")
}

func TestNames(t *testing.T) {
	assert.Equal(t, "A", colName(0))
	assert.Equal(t, "Z", colName(25))
	assert.Equal(t, "AA", colName(26))
	assert.Equal(t, "AZ", colName(51))
	used := map[string]bool{}
	assert.Equal(t, "Q1 2026 Revenue", sheetName("Q1/2026:Revenue", used), "characters Excel refuses in a sheet name")
	assert.Len(t, []rune(sheetName(strings.Repeat("x", 40), used)), 31)
	assert.Equal(t, "ab", escape("a\x00b"), "characters XML cannot hold are left out")
	assert.Equal(t, "'@SUM(A1)", SafeCell("@SUM(A1)"))
	assert.Equal(t, "Plain", SafeCell("Plain"))
}
