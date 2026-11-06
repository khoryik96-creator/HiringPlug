package cvformats

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

// docx makes a minimal .docx around a document body.
func docx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<w:document xmlns:w="x"><w:body>` + body + `</w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// part returns a part of a .docx.
func part(t *testing.T, file []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			b, err := io.ReadAll(rc)
			require.NoError(t, err)
			return string(b)
		}
	}
	t.Fatalf("no %s", name)
	return ""
}

// lines is the text of each paragraph.
func lines(xml string) []string {
	xml = strings.ReplaceAll(xml, "<w:br/>", "<w:t>\n</w:t>")
	var out []string
	for _, s := range segments(xml) {
		if s.para {
			out = append(out, paragraphText(s.xml))
		}
	}
	return out
}

var siti = Candidate{
	FirstName: "Siti", LastName: "Aisyah", Email: ptr("siti@example.com"), Phone: ptr("+60123456789"),
	Nationality: ptr("Malaysian"), Location: ptr("Kuala Lumpur"), CurrentTitle: ptr("Senior Accountant"), CurrentCompany: ptr("Maju Bhd"),
	NoticeValue: ptr(1), NoticeUnit: "month", WorkAuthorization: ptr("local"), Skills: []string{"IFRS", "SAP"},
	Work: []Job{
		{Title: ptr("Senior Accountant"), Company: ptr("Maju Bhd"), StartDate: ptr("2021-03"), Current: true, Description: ptr("Month-end close\nIFRS 16 & audits")},
		{Title: ptr("Accountant"), Company: ptr("Contoh <Holdings>"), StartDate: ptr("2017"), EndDate: ptr("2021-02")},
	},
	Education:      []Qualification{{Qualification: ptr("ACCA"), Institution: ptr("ACCA Global"), EndYear: ptr(2019)}},
	CurrentSalary:  &Salary{Min: "8500.00", Currency: "MYR", Period: "month"},
	ExpectedSalary: &Salary{Min: "9000.00", Max: ptr("10000.00"), Currency: "MYR", Period: "month"},
}

func TestTheStandardTemplate(t *testing.T) {
	found, err := Inspect(Standard())
	require.NoError(t, err)
	assert.Empty(t, CheckTemplate(found), "the standard template uses only known placeholders")

	ctx := Context{AgencyName: "Contoh Recruitment", RecruiterName: "Aina", RecruiterEmail: "aina@contoh.my", Today: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), JobTitle: "Finance Manager"}
	out, err := Render(Standard(), Values(siti, ctx))
	require.NoError(t, err)
	doc := part(t, out, "word/document.xml")
	text := strings.Join(lines(doc), "\n")
	assert.Contains(t, text, "Siti Aisyah\nSenior Accountant\nPosition: Finance Manager\nProfile")
	assert.NotContains(t, text, "Client:", "no client: the line goes")
	assert.NotContains(t, text, "Email", "contact details hidden unless asked")
	assert.NotContains(t, text, "salary", "salary only when asked")
	assert.NotContains(t, text, "Summary", "an empty section goes with its heading")
	assert.Contains(t, text, "Skills\nIFRS, SAP")
	assert.Contains(t, text, "Experience\nSenior Accountant\nMaju Bhd\nMar 2021 to Present\nMonth-end close\nIFRS 16 & audits\nAccountant\nContoh <Holdings>\n2017 to Feb 2021")
	assert.Contains(t, text, "Education\nACCA\nACCA Global\n2019")
	assert.NotContains(t, text, "{{")
	assert.Contains(t, doc, "Contoh &lt;Holdings&gt;", "values are escaped")
	assert.Contains(t, doc, `Month-end close</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Normal"/></w:pPr><w:r><w:t xml:space="preserve">IFRS 16 &amp; audits`,
		"each duty its own paragraph, in the paragraph's style")
	assert.Contains(t, lines(part(t, out, "word/footer1.xml"))[0], "Presented by Contoh Recruitment: Aina, aina@contoh.my · 3 November 2026")

	ctx.ShowContact, ctx.IncludeSalary, ctx.Summary = true, true, "Strong IFRS background."
	out, err = Render(Standard(), Values(siti, ctx))
	require.NoError(t, err)
	text = strings.Join(lines(part(t, out, "word/document.xml")), "\n")
	assert.Contains(t, text, "Email: siti@example.com\nPhone: +60123456789\nCurrent salary: MYR 8,500 a month\nExpected salary: MYR 9,000 – 10,000 a month")
	assert.Contains(t, text, "Summary\nStrong IFRS background.")
}

func TestWordSplitsPlaceholdersOverRuns(t *testing.T) {
	tpl := docx(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Name: {{cand</w:t></w:r><w:r><w:t>idate.fullName</w:t></w:r><w:r><w:t xml:space="preserve">}} </w:t></w:r></w:p>`+
		`<w:p><w:pPr><w:pStyle w:val="x"/></w:pPr><w:r><w:t>Plain text</w:t></w:r></w:p>`+
		`<w:tbl><w:tr><w:tc><w:tcPr/><w:p><w:r><w:t>{{candidate.email}}</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p/>`)
	out, err := Render(tpl, Values(siti, Context{}))
	require.NoError(t, err)
	doc := part(t, out, "word/document.xml")
	assert.Contains(t, doc, `<w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">Name: Siti Aisyah </w:t></w:r><w:r><w:t></w:t></w:r>`, "joined into the first run, keeping its formatting")
	assert.Contains(t, doc, "Plain text")
	assert.Contains(t, doc, "<w:tcPr/><w:p/></w:tc>", "a table cell keeps a paragraph")
}

// A branded template's summary box: a text box (a drawing anchored in a
// paragraph) with a label and a placeholder, Word's modern copy and its
// fallback. Each of the box's paragraphs is filled on its own: the label
// stays, the drawing is not broken up, and an empty value leaves the box.
func TestTextBoxesAreFilledParagraphByParagraph(t *testing.T) {
	box := `<w:txbxContent><w:p><w:r><w:t>ABOUT HIM / HER</w:t></w:r></w:p><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>{{summary}}</w:t></w:r></w:p></w:txbxContent>`
	tpl := docx(t, `<w:p><w:r><w:t>Anchor </w:t></w:r><w:r><mc:AlternateContent xmlns:mc="m"><mc:Choice><w:drawing><wps:txbx xmlns:wps="s">`+box+
		`</wps:txbx></w:drawing></mc:Choice><mc:Fallback><w:pict><v:textbox xmlns:v="v">`+box+`</v:textbox></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>`+
		`<w:p><w:r><w:t>{{candidate.fullName}}</w:t></w:r></w:p>`)
	out, err := Render(tpl, Data{Values: map[string]string{"summary": "Ten years in audit.", "candidate.fullName": "Siti Aisyah"}})
	require.NoError(t, err)
	doc := part(t, out, "word/document.xml")
	assert.Equal(t, 2, strings.Count(doc, `<w:p><w:r><w:t>ABOUT HIM / HER</w:t></w:r></w:p>`), "the label stays in both copies")
	assert.Equal(t, 2, strings.Count(doc, `<w:rPr><w:b/></w:rPr><w:t xml:space="preserve">Ten years in audit.</w:t>`), "the summary, once in each copy, with its formatting")
	assert.Contains(t, doc, `<w:r><w:t>Anchor </w:t></w:r><w:r><mc:AlternateContent`, "the anchor's own text and the drawing are untouched")
	assert.Contains(t, doc, "Siti Aisyah")

	out, err = Render(tpl, Data{Values: map[string]string{"candidate.fullName": "Siti Aisyah"}})
	require.NoError(t, err)
	doc = part(t, out, "word/document.xml")
	assert.Contains(t, doc, "<w:drawing>", "no summary: the box stays (a text box needs its paragraphs)")
	assert.Equal(t, 2, strings.Count(doc, "ABOUT HIM / HER"))
}

func TestBadTemplates(t *testing.T) {
	_, err := Inspect([]byte("not a zip"))
	assert.ErrorContains(t, err, "not a Word")
	_, err = Inspect(docx(t, `<w:p><w:r><w:t>{{#experience}}</w:t></w:r></w:p>`))
	assert.ErrorContains(t, err, "not closed")
	_, err = Inspect(docx(t, `<w:p><w:r><w:t>Jobs: {{#experience}}</w:t></w:r></w:p>`))
	assert.ErrorContains(t, err, "alone in its paragraph")
	found, err := Inspect(docx(t, `<w:p><w:r><w:t>{{candidate.salary}} {{candidate.fullName}}</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>{{#experience}}</w:t></w:r></w:p><w:p><w:r><w:t>{{title}} {{grade}}</w:t></w:r></w:p><w:p><w:r><w:t>{{/experience}}</w:t></w:r></w:p>`))
	require.NoError(t, err)
	assert.Equal(t, "unknown placeholders: {{candidate.salary}}, {{grade}}", CheckTemplate(found))
}

func TestValues(t *testing.T) {
	v := Values(siti, Context{}).Values
	assert.Equal(t, "S.A.", v["candidate.initials"])
	assert.Equal(t, "1 month", v["candidate.noticePeriod"])
	assert.Equal(t, "Local", v["candidate.workAuthorization"])
	assert.Equal(t, "MYR 1,234,567.50 a year", Money(&Salary{Min: "1234567.50", Currency: "MYR", Period: "year"}))
	assert.Equal(t, "From Jan 2020", Dates(Job{StartDate: ptr("2020-01")}))
	assert.Equal(t, "Siti Aisyah - CV (Contoh).docx", FileName(siti, "Contoh", false))
}
