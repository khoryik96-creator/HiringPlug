package industries

import (
	"encoding/json"
	"regexp"
	"sync"
)

var seedGroups = sync.OnceValue(func() []seedCategory {
	var out []seedCategory
	if err := json.Unmarshal(seedData, &out); err != nil {
		panic(err)
	}
	return out
})

type employerRule struct {
	label   string
	pattern *regexp.Regexp
}

func rule(label, pattern string) employerRule {
	return employerRule{label, regexp.MustCompile("(?i)" + pattern)}
}

// Explicit business words or known organisations, in the selected employer name only.
var employerRules = []employerRule{
	rule("Government Sector", `\b(bank negara|mdec|ministry|kementerian|jabatan|government|kerajaan)\b`),
	rule("FSI – Banking", `\b(maybank|cimb|public bank|rhb|hong leong bank|ambank|affin bank|alliance bank|bank islam|bank rakyat|ocbc|uob|hsbc|standard chartered|citibank|agrobank|bank)\b`),
	rule("FSI - Insurance", `\b(allianz|prudential|great eastern|etiqa|zurich insurance|manulife|insurance|takaful)\b`),
	rule("Tech - Hardware", `\b(dell|hewlett.packard|lenovo)\b`),
	rule("Tech – ERP/CRM/HCM/Application", `\b(sap|oracle)\b|\berp (consultancy|consulting|solutions)\b`),
	rule("Information Technology & Services", `\b(microsoft|ibm|software|it services|systems integrator)\b`),
	rule("Industrial/Manufacturing", `\b(intel|infineon|micron|western digital|inari|vitrox|proton|perodua|petronas|exxon|tenaga|semiconductor|manufacturing)\b`),
	rule("FMCG", `\b(nestl[eé]|unilever|procter|dutch lady|coca.cola|pepsi|mondelez|fmcg)\b`),
	rule("Digital & E-Commerce", `\b(shopee|lazada|grab|foodpanda|zalora|carousell)\b`),
	rule("Professional Services", `\b(deloitte|pwc|pricewaterhouse|ernst|kpmg|grant thornton|bdo|baker tilly|mckinsey|boston consulting)\b`),
	rule("Life Science/Medical", `\b(ihh|kpj|pantai|gleneagles|columbia asia|hospital|pharmaceutical|clinic)\b`),
	rule("Property & Construction", `\b(gamuda|ijm|sp setia|mah sing|eco world|construction|real estate)\b`),
	rule("Education", `\b(university|college|school|polytechnic|politeknik)\b`),
}
