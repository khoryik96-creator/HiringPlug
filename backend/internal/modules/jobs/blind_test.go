package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func str(s string) *string { return &s }

var maybank = Client{Name: "Maybank Berhad", Website: "https://www.maybank2u.com.my/careers", Contacts: []string{"Lim Wei", "Lim", "Wei"}}

func TestClientNames(t *testing.T) {
	assert.Equal(t, []string{"maybank2u.com.my", "Maybank Berhad", "maybank2u", "Maybank"}, ClientNames(maybank))
	assert.Equal(t, []string{"Contoh Teknologi"}, ClientNames(Client{Name: "Contoh Teknologi Sdn. Bhd."})[1:], "legal endings are dropped")
	assert.Empty(t, ClientNames(Client{Name: "AB"}), "too short to look for")
	assert.Equal(t, []string{"careers.acme.com.my", "acme"}, ClientNames(Client{Website: "https://careers.acme.com.my/jobs"}),
		"the name part of the address, not a subdomain like careers")
	assert.Equal(t, []string{"jobs.example.xyz"}, ClientNames(Client{Website: "https://jobs.example.xyz"}), "unknown ending: only the whole address")
}

func TestBlindAd(t *testing.T) {
	ad := AdDetails{
		Title: "Credit Analyst, Maybank",
		Description: str("Maybank Berhad is hiring.\nAt maybank's KL office you will:\n- review loans for MAYBANK clients\n" +
			"Apply at www.maybank2u.com.my or email lim.wei@maybank.com, call +60 3-2070 8833. Ask for Lim Wei."),
		ClientDescription: str("a leading Malaysian bank"),
		Location:          str("Menara Maybank, Kuala Lumpur"),
		SalaryText:        str("Maybank grade 42"),
	}
	b := Blind(ad, maybank)
	assert.Equal(t, "Credit Analyst, our client", b.Title, "a title never takes the description")
	assert.Equal(t, "A leading Malaysian bank is hiring.\nAt our client's KL office you will:\n- review loans for our client clients\n"+
		"Apply at [link removed] or email [email removed], call [phone removed]. Ask for the hiring manager.", *b.Description)
	assert.Equal(t, "Menara our client, Kuala Lumpur", *b.Location, "the location may name the client too")
	assert.Equal(t, "Our client grade 42", *b.SalaryText)
	assert.False(t, NamesClient(b.Title+*b.Description+*b.Location+*b.SalaryText, maybank))
	assert.True(t, NamesClient("Join maybank2u today", maybank))

	ad.ClientDescription = nil
	assert.Contains(t, *Blind(ad, maybank).Description, "Our client is hiring.", "capitalised at the start of a sentence")
	assert.Nil(t, Blind(AdDetails{Title: "Clerk"}, maybank).Description)
}

func TestBackToBackMentionsAreAllReplaced(t *testing.T) {
	b := Blind(AdDetails{Title: "Maybank/Maybank Islamic", Description: str("MAYBANK MAYBANK")}, maybank)
	assert.Equal(t, "Our client/our client Islamic", b.Title)
	assert.Equal(t, "Our client our client", *b.Description)
}

func TestSourceHashFollowsTheAdsWords(t *testing.T) {
	a := AdDetails{Title: "Clerk", Description: str("Filing.")}
	b := a
	b.SalaryText = str("RM 3,000")
	assert.Equal(t, SourceHash(a), SourceHash(b), "only the title and text matter")
	b.Description = str("Filing and typing.")
	assert.NotEqual(t, SourceHash(a), SourceHash(b))
}

func TestAdDocAndFileName(t *testing.T) {
	d := AdDetails{Location: str("Kuala Lumpur"), Country: str("MY"), SalaryText: str("RM 8,000")}
	doc := AdDoc("Finance Manager", str("Lead the team.\n\n- Close the books\n- Report"), d, "Full time", "")
	assert.Equal(t, "Kuala Lumpur, Malaysia · Full time · RM 8,000", doc[1].Text)
	assert.Len(t, doc, 5)
	assert.Equal(t, "Finance Manager (blind).pdf", ExportFileName("Finance Manager", "blind", "pdf"))
	assert.Equal(t, "C Developer.docx", ExportFileName("C++ / Developer", "original", "docx"))
}
