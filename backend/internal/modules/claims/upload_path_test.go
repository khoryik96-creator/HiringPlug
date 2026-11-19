package claims

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadPathIsMonthPersonReceiptType(t *testing.T) {
	got := UploadPath(Upload{
		Root: "Claims", Person: "Aisyah Rahman", ReceiptType: "Grab / e-hailing fare", Merchant: "Grab",
		Currency: "MYR", Amount: "23.00", ClaimID: "clm_01jbz6w0m0k8q9x2r3s4t5v6w7", FileName: "IMG_0042.JPG",
		ReceiptDate: *day("2026-11-03"),
	})
	assert.Equal(t, []string{"Claims", "2026-11", "Aisyah Rahman", "Grab - e-hailing fare", "2026-11-03 Grab MYR 23.00 (t5v6w7).jpg"}, got,
		"a slash in a label is not a folder")
}

func TestUploadPathNamesAreSafeOnEveryDrive(t *testing.T) {
	got := UploadPath(Upload{
		Root: " Finance / Claims/../ ", Person: "  ..Tan: \"Boss\"  ", ReceiptType: "", Merchant: "A*B?<C>|D\tE",
		Currency: "SGD", Amount: "8.50", ClaimID: "clm_01jbz6w0m0k8q9x2r3s4t5v6w7", FileName: "receipt", ContentType: "application/pdf",
		ReceiptDate: *day("2026-01-31"),
	})
	assert.Equal(t, []string{"Finance", "Claims", "2026-01", "Tan- -Boss-", "Other", "2026-01-31 A-B--C--D E SGD 8.50 (t5v6w7).pdf"}, got)
	long := UploadPath(Upload{Root: "Claims", Person: strings.Repeat("Nur ", 40), Merchant: strings.Repeat("Shop ", 20),
		Currency: "MYR", Amount: "1.00", ClaimID: "clm_x", ReceiptDate: *day("2026-02-01")})
	for _, s := range long {
		assert.LessOrEqual(t, len([]rune(s)), segmentMax, s)
		assert.Equal(t, strings.TrimSpace(s), s)
	}
	assert.Equal(t, "Claims", UploadPath(Upload{Root: "/ . /", ReceiptDate: *day("2026-02-01")})[0], "no usable folder: the default")
}

func TestValidRoot(t *testing.T) {
	assert.True(t, ValidRoot("Claims"))
	assert.True(t, ValidRoot("Finance/Claims 2026"))
	assert.False(t, ValidRoot(" / .. / "))
	assert.False(t, ValidRoot("a/b/c/d/e/f"), "six deep is too deep")
}
