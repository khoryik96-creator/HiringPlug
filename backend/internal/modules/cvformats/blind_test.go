package cvformats

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrub(t *testing.T) {
	id := IdentityOf(Candidate{FirstName: "Ahmad", LastName: "bin Ali", Email: ptr("Ahmad.Ali@Mail.com"), Phone: ptr("+60 12-345 6789")})
	assert.Equal(t, []string{"Ahmad", "Ali"}, id.Names, "bin is a connector, not a name")
	got := Scrub("Ahmad's team grew under Ali. Contact ahmad.ali@mail.com or +60 12-345 6789, linkedin.com/in/ahmadali, www.ahmad.my. Saved RM 1,250,000 in 2019-2021.", id)
	assert.Equal(t, "the candidate's team grew under the candidate. Contact [email removed] or [phone removed], [link removed] [link removed] Saved RM 1,250,000 in 2019-2021.", got)
	assert.Equal(t, "Alibaba Cloud", Scrub("Alibaba Cloud", id), "whole words only")
}

func TestBlindCv(t *testing.T) {
	c := siti
	c.ID = "cand_01jbz6w0m0k8q9x2r3s4t5v6w7"
	c.Work = append([]Job{{Title: ptr("Lead"), Company: ptr("Maju"), Description: ptr("Siti built the team; call 012-345 6789")}}, c.Work...)
	data := Blind(Values(c, Context{ShowContact: true, Summary: "Siti Aisyah is strong.", Today: time.Now()}), c, Reference(c.ID))
	out, err := Render(Standard(), data)
	require.NoError(t, err)
	text := strings.Join(lines(part(t, out, "word/document.xml")), "\n")
	assert.Contains(t, text, "Candidate "+Reference(c.ID))
	assert.NotContains(t, text, "Siti")
	assert.NotContains(t, text, "Aisyah")
	assert.NotContains(t, text, "siti@example.com")
	assert.NotContains(t, text, "Malaysian", "no nationality")
	assert.Contains(t, text, "the candidate built the team; call [phone removed]")
	assert.Contains(t, text, "Summary\nthe candidate is strong.")
	leaks, err := Leaks(out, IdentityOf(c))
	require.NoError(t, err)
	assert.Empty(t, leaks)
	assert.Equal(t, "Contoh CV - Candidate "+Reference(c.ID)+" (Blinded).docx", FileName(c, "Contoh", true))
}

func TestLeaksAreFoundAnywhere(t *testing.T) {
	id := IdentityOf(siti)
	leaks, err := Leaks(docx(t, `<w:p><w:r><w:t>Prepared for SITI by email siti@example.com, tel 6012 345 6789</w:t></w:r></w:p>`), id)
	require.NoError(t, err)
	assert.Equal(t, []string{"name", "email", "phone"}, leaks)
	leaks, err = Leaks(docx(t, `<w:p><w:r><w:t>2017 2021 1234 5678 Sitiawan</w:t></w:r></w:p>`), IdentityOf(Candidate{FirstName: "Siti", Phone: ptr("1234 5678")}))
	require.NoError(t, err)
	assert.Equal(t, []string{"phone"}, leaks, "a town that starts with the name is not the name; the number is the number")
	leaks, err = Leaks(docx(t, `<w:p><w:r><w:t>2017 2021 2022 2023</w:t></w:r></w:p>`), IdentityOf(Candidate{FirstName: "X", Phone: ptr("20212022")}))
	require.NoError(t, err)
	assert.Empty(t, leaks, "years next to each other are not a phone number")
}

func TestLeaksReadTheFileAsWordShowsIt(t *testing.T) {
	id := IdentityOf(Candidate{FirstName: "May", LastName: "Tan Siew Ling", Email: ptr("may@example.com")})
	leaks, err := Leaks(docx(t, `<w:p><w:r><w:t>Si</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">ew, auditor</w:t></w:r></w:p>`), id)
	require.NoError(t, err)
	assert.Equal(t, []string{"name"}, leaks, "a name Word split across runs")
	leaks, err = Leaks(docx(t, `<w:p><w:r><w:t>Audit Senior, Jan 2020 - May 2023; based in Petaling Jaya</w:t></w:r></w:p>`), id)
	require.NoError(t, err)
	assert.Empty(t, leaks, "a month, not the candidate: the blind CV is saved")
}
