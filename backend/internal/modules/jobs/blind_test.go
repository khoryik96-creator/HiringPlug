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

func TestTheJDsOwnWayOfWritingTheNameIsBlindedWhole(t *testing.T) {
	jd := str("About Maju Foods Berhad\nMaju Foods Berhad is one of Malaysia's leading FMCG makers. Maju Foods (M) Sdn. Bhd. is hiring.")
	for _, record := range []string{"Maju Foods Berhad", "MAJU FOODS BHD", "Maju Foods Sdn Bhd", "Maju"} {
		b := Blind(AdDetails{Title: "Finance Manager", Description: jd}, Client{Name: record})
		assert.Equal(t, "About our client\nOur client is one of Malaysia's leading FMCG makers. Our client is hiring.",
			*b.Description, "client on record: %s", record)
	}
	b0 := Blind(AdDetails{Title: "Analyst", Description: str("Maju is a co-founder of the guild.")}, Client{Name: "Maju"})
	assert.Equal(t, "Our client is a co-founder of the guild.", *b0.Description, "no ending taken from lowercase words")
	b := Blind(AdDetails{Title: "Analyst", Description: str("Maju Holdings Berhad owns Maju.")}, Client{Name: "Maju"})
	assert.Equal(t, "Our client owns our client.", *b.Description, "a word before the ending is the company's name")
	b = Blind(AdDetails{Title: "Analyst", Description: str("Maju and Contoh Bhd are partners.")}, Client{Name: "Maju"})
	assert.Equal(t, "Our client and Contoh Bhd are partners.", *b.Description, "lowercase words are not taken")
}

func TestTheClientsShortNameAndPluralPossessiveAreBlinded(t *testing.T) {
	jd := str("About MAJU FOODS BHD\nMaju Foods (M) Sdn. Bhd. is a food maker.\n- Lead HR for Maju Foods' plants\n- Report to Maju's HR Director\n- MAJU culture: kita maju bersama")
	b := Blind(AdDetails{Title: "HR Manager", Description: jd}, Client{Name: "Maju Foods Berhad"})
	assert.Equal(t, "About our client\nOur client is a food maker.\n- Lead HR for our client's plants\n- Report to our client's HR Director\n- Our client culture: kita maju bersama",
		*b.Description, "the Malay word maju stays")
	assert.True(t, NamesClient("Join Maju today", Client{Name: "Maju Foods Berhad"}))

	t.Run("a common first word is not the company", func(t *testing.T) {
		b := Blind(AdDetails{Title: "HR Manager", Description: str("Global Logistics Sdn Bhd hires a Global Head of HR.")}, Client{Name: "Global Logistics Sdn Bhd"})
		assert.Equal(t, "Our client hires a Global Head of HR.", *b.Description)
		assert.Empty(t, ShortName(Client{Name: "Maju"}), "a one-word name is already matched whole")
		assert.Empty(t, ShortName(Client{Name: "ABC Sdn Bhd"}), "too short to stand alone")
	})

	t.Run("a closing quote is not a possessive", func(t *testing.T) {
		b := Blind(AdDetails{Title: "HR Manager", Description: str("Join 'Maju' today.")}, Client{Name: "Maju"})
		assert.Equal(t, "Join 'our client' today.", *b.Description)
	})
}

func TestADescriptionMidSentenceStartsInLowerCase(t *testing.T) {
	b := Blind(AdDetails{Title: "HR", Description: str("About Maju\nMaju is hiring.")}, Client{Name: "Maju", Website: ""})
	assert.Equal(t, "About our client\nOur client is hiring.", *b.Description)
	d := "A leading food maker"
	b = Blind(AdDetails{Title: "HR", Description: str("About Maju\nMaju is hiring."), ClientDescription: &d}, Client{Name: "Maju"})
	assert.Equal(t, "About a leading food maker\nOur client is hiring.", *b.Description)
	d = "Leading food maker"
	b = Blind(AdDetails{Title: "HR", Description: str("Join Maju."), ClientDescription: &d}, Client{Name: "Maju"})
	assert.Equal(t, "Join Leading food maker.", *b.Description, "only an article is lowered")
}
