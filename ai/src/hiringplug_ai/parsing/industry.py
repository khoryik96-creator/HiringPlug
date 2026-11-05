"""The industry of a candidate's current (latest) employer, in Kano's
industries, inferred from the employer's name first and the job title
next. Like Kano's local rules; Kano asks an AI model when they find nothing,
which HiringPlug does once its AI is on. The quote is the words the industry
was inferred from, so a person can see why."""

from __future__ import annotations

import re

# Known names first (across every industry), then general words: "Infineon
# Technologies" is a manufacturer by its name, whatever "Technologies" says.
NAMES: list[tuple[str, str]] = [
    (
        "Financial Services",
        r"maybank|cimb|public bank|rhb|hong leong|ambank|affin|alliance bank|bank islam|bank rakyat|"
        r"ocbc|uob|hsbc|standard chartered|citibank|\bciti\b|\bbsn\b|agrobank|allianz|\baia\b|prudential|"
        r"great eastern|etiqa|zurich|manulife",
    ),
    (
        "Digital & E-Commerce",
        r"shopee|lazada|\bgrab\b|foodpanda|touch ?'?n ?go|\bboost\b|zalora|carousell|mudah",
    ),
    (
        "Professional Services",
        r"deloitte|pwc|pricewaterhouse|\bey\b|ernst ?& ?young|kpmg|grant thornton|\bbdo\b|"
        r"baker tilly|crowe|accenture|mckinsey|boston consulting",
    ),
    ("Life Science/Medical", r"\bihh\b|\bkpj\b|pantai|gleneagles|columbia asia|sunway medical"),
    (
        "Information Technology & Services",
        r"\bsap\b|oracle|microsoft|\bibm\b|maxis|celcom|\bdigi\b|telekom|\btm\b|u mobile",
    ),
    ("Property & Construction", r"gamuda|\bijm\b|sp setia|mah sing|eco world"),
    (
        "Industrial/Manufacturing",
        r"\bintel\b|infineon|micron|western digital|inari|vitrox|proton|perodua|petronas|"
        r"\bshell\b|exxon|tenaga|\btnb\b",
    ),
    (
        "FMCG",
        r"nestl[eé]|unilever|procter|p ?& ?g|f ?& ?n|dutch lady|coca-?cola|pepsi|mondelez|aeon|mydin|"
        r"speedmart|watsons|guardian",
    ),
    ("Government Sector", r"bank negara|\bmdec\b"),
]
WORDS: list[tuple[str, str]] = [
    (
        "Financial Services",
        r"\bbank|insurance|takaful|assurance|asset management|securities|investment bank|"
        r"capital markets|private equity|venture capital|finance berhad",
    ),
    ("Digital & E-Commerce", r"e-?commerce|marketplace|fintech|payments?|e-?wallet"),
    (
        "Professional Services",
        r"consult|advisory|recruit|staffing|advertising|law firm|advocates|solicitors|"
        r"chartered accountants|audit|outsourcing",
    ),
    (
        "Life Science/Medical",
        r"hospital|medical|clinic|health ?care|pharma|biotech|life science|diagnostic|laborator",
    ),
    (
        "Information Technology & Services",
        r"software|technolog|\bit\b services|cloud|saas|digital solutions|"
        r"data cent(?:re|er)|cyber|telecom|\bnetworks?\b",
    ),
    (
        "Property & Construction",
        r"propert|construction|builders|real estate|realty|architect|hotel|resort|"
        r"hospitality|tourism",
    ),
    (
        "Industrial/Manufacturing",
        r"manufactur|semiconductor|electronics|automotive|oil ?& ?gas|petroleum|chemical|"
        r"plantation|logistic|freight|shipping|energy|"
        r"power (?:plant|station|generation)|utilities|industries|steel|rubber|gloves",
    ),
    ("FMCG", r"\bfood|beverage|consumer goods|fmcg|retail|fashion|apparel|cosmetic|luxury"),
    ("Education", r"universit|college|school|academy|education|polytechnic|politeknik"),
    (
        "Government Sector",
        r"ministry|kementerian|jabatan|government|kerajaan|majlis|dewan bandaraya|statutory body",
    ),
]
# Kano's industries, the values an industry may take.
RULES_INDUSTRIES = [
    "Digital & E-Commerce", "Financial Services", "FMCG", "Industrial/Manufacturing",
    "Information Technology & Services", "Life Science/Medical", "Property & Construction",
    "Professional Services", "Education", "Government Sector",
]  # fmt: skip
_COMPILED = [(name, re.compile(rf"(?i)({pattern})")) for name, pattern in NAMES + WORDS]


def classify(text: str) -> str | None:
    for name, pattern in _COMPILED:
        if pattern.search(text):
            return name
    return None


# A job title says little about the employer's industry, except IT work (as
# Kano's ERP and application-support rules): "Java Developer" is no property
# developer, "Power BI Analyst" no power plant.
_IT_TITLE = re.compile(
    r"(?i)\b(?:sap|erp|oracle|crm|software|developer|programmer|devops|data engineer|it support|"
    r"network engineer|system administrator|sysadmin|cloud|power bi|full[- ]stack|front[- ]end|back[- ]end)\b"
)


def industry(company: str | None, title: str | None) -> tuple[str, str] | None:
    """(industry, quote): from the employer's name, else IT work in its job title."""
    if company and (found := classify(company)):
        return found, company
    if title and _IT_TITLE.search(title):
        return "Information Technology & Services", title
    return None
