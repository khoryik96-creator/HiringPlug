"""Job families as a recruiter knows them, as data (recruiter.py reads titles
with them).

FAMILIES: what puts a title in a family (a pattern on the title spelt out,
in lower case) and what the work is, in plain words. A title in a family is
read by the meaning model as that work, so "Accounts Executive" (accounting)
and "Account Executive" (left to the work under it: sales in most CVs,
accounts in some) are no longer near twins because they share a word.

WORK_SIGNS: what a person's work says about their family when their title
cannot ("Principal Consultant" placing accountants is a recruiter).

Tuned on two recruiter benches (made-up candidates and the searches a
recruiter types, with the traps: Account vs Accounts Executive, talent vs
procurement sourcing, software vs factory QA, Finance Business Partner,
Company Secretary); see tests/test_meaning_bench.py.
"""

from __future__ import annotations

import re

FAMILIES: list[tuple[str, str, str]] = [
    # People
    (
        "recruitment",
        r"recruit|talent acquisition|headhunt|executive search|resourcing|talent sourc|sourcer|staffing|"
        r"talent (partner|advisor|specialist|lead|scout|hunter)",
        "recruitment: finding, interviewing and hiring people for jobs",
    ),
    (
        "hr business partner",
        r"(hr|human resources|people) business partner|people partner|\bhrbp\b",
        "human resources business partner: advising managers on people, performance and organisation",
    ),
    ("payroll", r"payroll", "payroll: paying salaries and statutory contributions such as EPF and SOCSO"),
    (
        "reward",
        r"compensation|benefits|reward",
        "compensation and benefits: salary structures, bonuses and benefits",
    ),
    (
        "learning",
        r"learning and development|training and development|\btrainer\b|\bl&d\b",
        "training and developing employees",
    ),
    (
        "hr",
        r"human resources|\bpersonnel\b|people operations|employee relations|\bhr\b",
        "human resources: staff records, policies, onboarding and employee matters",
    ),
    # Money
    (
        "erp finance",
        r"\b(sap|erp|oracle)\b.*\b(fi|co|fico|finance|financials)\b|\bfi co\b",
        "ERP finance systems consulting: SAP FI/CO, Oracle Financials",
    ),
    ("erp", r"\bsap\b|\berp\b|oracle (financials|ebs)|dynamics|s/4 ?hana", "ERP business systems consulting"),
    (
        "finance management",
        r"finance manager|financial controller|chief financial|head of finance|finance director|"
        r"financial planning|\bfp&a\b|\bcfo\b|finance business partner|financial analyst|finance analyst|"
        r"management account",
        "finance management: budgets, forecasts, financial reporting and business performance",
    ),
    ("audit", r"audit", "auditing financial statements and internal controls"),
    ("tax", r"\btax(?:ation|es)?\b", "tax: corporate and personal tax returns and planning"),
    ("treasury", r"treasury", "treasury: cash, banking and funding"),
    (
        "investment",
        r"investment|equity research|portfolio manager|fund manager|asset management|private equity|"
        r"venture capital",
        "investment analysis: valuing companies and managing funds",
    ),
    (
        "accounting",
        r"accountant|accounts|\baccounting\b|account assistant|bookkeep|general ledger|accounts payable|"
        r"accounts receivable|akauntan|cost account|finance (executive|officer|assistant|clerk)|"
        r"credit control|"
        r"\bfinance\b",
        "accounting: bookkeeping, ledgers, reconciliations, invoices and financial statements",
    ),
    (
        "banking credit",
        r"\bcredit (analyst|officer|manager|executive|evaluation|risk)|\bloans?\b|underwrit|mortgage",
        "bank credit: assessing loan applications and lending risk",
    ),
    (
        "banking",
        r"relationship manager|\bbanker\b|private banking|wealth|branch manager|\bteller\b|\bbanking\b|"
        r"trade finance|cash management|financial (?:planner|adviser|advisor)",
        "banking: looking after bank clients, their accounts, loans and investments",
    ),
    ("risk", r"\brisk\b", "risk management: identifying and controlling business and financial risks"),
    (
        "compliance",
        r"compliance|\baml\b|anti-money|regulatory|\bkyc\b|know your customer|sanctions|fraud|"
        r"financial crime|data protection|privacy|\bpdpa\b|\bgdpr\b|\bdpo\b",
        "compliance: regulations, anti-money laundering and controls",
    ),
    (
        "insurance",
        r"insurance|actuar|claims (executive|officer|adjuster)|underwriter",
        "insurance: policies, claims and underwriting",
    ),
    # Selling and customers
    ("customer success", r"customer success", "customer success: keeping and growing existing customers"),
    (
        "sales",
        r"\bsales\b|business development|account manager|key account|territory|inside sales|channel sales|"
        r"\bkam\b|partnerships?|alliances|pre-?sales",
        "sales: selling to customers, winning new business and managing client accounts",
    ),
    (
        "customer service",
        r"customer service|call cent|contact cent|customer support|customer care",
        "customer service: answering customer calls, emails and complaints",
    ),
    (
        "retail",
        r"\bstore\b|\bretail\b|\boutlet\b|merchandis|cashier|shop (manager|assistant)|boutique",
        "retail: running shops, sales floors, stock and store staff",
    ),
    (
        "hospitality",
        r"(?:f&b|banquet|restaurant) servers?|\bchef\b|\bcook\b|kitchen|\bf&b\b|food and beverage|restaurant|"
        r"\bhotel\b|housekeeping|front office|barista|banquet|concierge|waiter|waitress|"
        r"guest (?:experience|relations|services)",
        "hospitality: hotels, restaurants, kitchens and guest service",
    ),
    # Technology
    (
        "software testing",
        r"software test|\btester\b|test automation|software quality|(qa|quality assurance) automation|"
        r"automation (tester|test engineer|qa)|"
        r"\bsdet\b|engineer in test",
        "software testing: testing applications and finding bugs",
    ),
    (
        "cyber security",
        r"cyber|information security|infosec|"
        r"(information technology|network|cloud|application|data|product|endpoint|offensive) security|"
        r"security (engineer|analyst|architect|consultant|specialist|operations|researcher|tester|auditor)|"
        r"\bsoc\b|penetration test(er|ing)?|pen ?test(er|ing)?|ethical hack|red team|blue team|\bgrc\b|"
        r"threat|vulnerabilit|incident response|identity and access|\biam\b|\bsiem\b|appsec|"
        r"chief information security",
        "cyber security: protecting systems and data, security monitoring, penetration testing and controls",
    ),
    (
        "data",
        # A BI or dashboards developer is a data role, though "developer" ends the title
        # (first: at the same place the longer reading wins).
        r"(power bi|tableau|etl|\bbi|business intelligence|data dashboards)( \w+){0,2} "
        r"(developer|engineer|analyst)|"
        r"\bdata\b(?! entry)|business intelligence|analytics|\bmis\b|management information|"
        r"reporting analyst|"
        r"machine learning|\bml\b|\bai\b|artificial intelligence|deep learning|computer vision|\bnlp\b|mlops",
        "data and analytics: analysing data and building reports and dashboards",
    ),
    (
        "cloud and devops",
        r"devops|devsecops|site reliability|\bsre\b|cloud|platform engineer|solutions? architect|"
        r"infrastructure architect|kubernetes",
        "cloud and DevOps engineering: cloud platforms, deployment pipelines and system architecture",
    ),
    (
        "software",
        r"software|developer|programmer|full stack|backend|back end|frontend|front end|"
        r"web develop|mobile develop|\bsde\b|\bswe\b|coder|chief technology|tech(nical)? lead|"
        r"software architect|technical architect",
        "software development: writing code and building applications",
    ),
    (
        "it infrastructure",
        # An administrator of systems is IT, not office administration (the
        # longer match wins where both end).
        r"\bnetwork(s|ing)?\b(?! marketing)|firewall|cisco|sysadmin|"
        r"\b(?:systems?|network|database|linux|unix|windows|servers?|information technology|cloud|storage|"
        r"backup|vmware|citrix|exchange|active directory|microsoft 365|office 365|o365|sharepoint|lan|wan) "
        r"(?:admin|administrat(?:or|ion))s?\b|"
        r"infrastructure|data cent(re|er)|\bservers?\b",
        "IT infrastructure: networks, firewalls and servers",
    ),
    (
        "it support",
        r"it support|helpdesk|help desk|desktop support|service desk|technical support|"
        r"information technology( operations)?|\bict\b|(?:information and )?communications? technology|"
        r"service delivery|\bitil\b",
        "IT support: fixing computers, accounts and user problems",
    ),
    (
        "product management",
        r"product (manager|owner|management|lead|director)|scrum|agile coach",
        "product management: deciding what a digital product does and planning its features",
    ),
    (
        "business analysis",
        r"business analyst|systems analyst|business systems|requirements analyst",
        "business analysis: gathering requirements and designing system changes",
    ),
    (
        "design",
        r"designer|\bux\b|\bui\b|user experience|graphic|creative director|illustrat",
        "design: visual, product and user experience design",
    ),
    # Engineering and operations
    (
        "quantity surveying",
        r"quantity survey",
        "quantity surveying: construction costs, tenders and contracts",
    ),
    (
        "building services",
        r"mechanical and electrical|\bm&e\b|building services",
        "building services engineering: mechanical and electrical systems in buildings",
    ),
    (
        "civil engineering",
        r"\bcivil\b|structural|geotechnical|site engineer|construction",
        "civil engineering: designing and building roads, drainage, buildings and structures",
    ),
    (
        "oil and gas",
        # Not palm oil: a palm oil mill is a plantation's.
        r"drilling|petroleum|reservoir|offshore|subsea|(?<!palm )(?<!edible )\boil\b(?! palm)|upstream|"
        r"wellsite|completions|\bo ?& ?g\b",
        "oil and gas engineering: wells, drilling and production",
    ),
    (
        "plantation",
        r"plantation|palm oil|oil palm|(?<!real )\bestates?\b(?! agent)|agronom|agricultur|\bplanters?\b|"
        r"\bfarm(?:ing|s)?\b|horticultur|\bmills? (?:manager|engineer|executive|assistant|supervisor)",
        "plantation and agriculture: oil palm estates, palm oil mills, crops and agronomy",
    ),
    (
        "telecommunications",
        r"telecom|\btelco\b|\brf (?:engineer|planning|optimi[sz]ation)|radio (?:frequency|network|access)|"
        r"\b(?:4g|5g|lte)\b|transmission engineer|fib(?:re|er) optic|\bftth\b|"
        r"communications? (?:systems? )?engineer",
        "telecommunications engineering: mobile and fixed networks, radio, transmission and fibre",
    ),
    (
        "health and safety",
        r"\bhse\b|\behs\b|health safety|safety|\bosh\b|environment health",
        "health and safety at work: inspections, permits and incident prevention",
    ),
    (
        "mechanical engineering",
        r"mechanical|maintenance engineer",
        "mechanical engineering: machines, plant equipment and maintenance",
    ),
    (
        "electrical engineering",
        r"electrical|electronic|power (?:distribution|transmission|generation|systems?|plant)|substation|"
        r"switchgear|high voltage",
        "electrical and electronic engineering",
    ),
    (
        "manufacturing engineering",
        r"process engineer|production|manufacturing|industrial engineer|equipment engineer",
        "manufacturing engineering: production lines, yield and process improvement",
    ),
    (
        "manufacturing quality",
        r"quality control|qa / qc|qa/qc|\bqc\b|haccp|"
        r"quality assurance (executive|officer|manager)",
        "quality assurance in factories: inspections, audits, ISO and food safety standards",
    ),
    (
        "procurement",
        r"procure|purchas|\bbuyer\b|strategic sourcing|sourcing manager|vendor management|"
        r"category manager",
        "procurement: buying from suppliers and negotiating prices",
    ),
    (
        "supply chain planning",
        r"supply chain|planner|demand planning|s&op|sales and operations planning",
        "supply chain planning: forecasting demand, supply and inventory",
    ),
    (
        "logistics",
        # Not an IT service's delivery, nor a power grid's distribution.
        r"logistic|shipping|freight|warehouse|forward|customs|transport|fleet|store ?keeper|"
        r"(?<!power )(?<!electricity )(?<!electrical )distribution|(?<!service )delivery",
        "logistics: shipments, warehouses, deliveries and transport",
    ),
    (
        "operations",
        r"\boperations\b|\bops\b|chief operating|\brevops\b",
        "operations: running a business's day-to-day processes, teams and service delivery",
    ),
    # Marketing, office, law, care, teaching
    (
        "digital marketing",
        r"digital marketing|performance marketing|social media|\bseo\b|search engine|"
        r"growth market|content market|e-?commerce market",
        "digital marketing: online advertising, social media and campaigns",
    ),
    (
        "marketing",
        r"marketing|\bbrand\b|\bcommunications?\b|public relations",
        "marketing: brands, campaigns, promotions and communications",
    ),
    (
        "corporate secretarial",
        r"company secretar|corporate secretar|\bcosec\b",
        "corporate secretarial: board meetings, statutory filings and company records",
    ),
    (
        "executive assistant",
        r"executive assistant|personal assistant|secretary",
        "executive assistant: supporting executives with diaries, travel, meetings and minutes",
    ),
    (
        "administration",
        # "admin" a whole word ("Database Administrator" is IT); an administrator
        # of something else is in its field ("Payroll Administrator").
        r"\badmin\b|\badministrative\b|administration|"
        r"^(?:(?:senior|junior|assistant|chief|principal|lead|head) )*administrator$|"
        r"(?:office|general|sales|clerical) administrator|office manager|receptionist|front desk|data entry|"
        r"clerical",
        "office administration: supplies, filing, facilities and office support",
    ),
    (
        "legal",
        r"legal|lawyer|counsel|paralegal|advocate|solicitor|litigation|conveyancing",
        "legal: contracts, legal advice and disputes",
    ),
    (
        "healthcare",
        r"nurs(e|ing)|doctor|physician|medical officer|pharmac|clinical|therapist|radiograph|dentist|"
        r"midwife|caregiver",
        "healthcare: caring for patients in hospitals, clinics and pharmacies",
    ),
    (
        "education",
        r"teach|lecturer|tutor|professor|academic|instructor|curriculum",
        "education: teaching students in schools, colleges and universities",
    ),
    (
        "project management",
        r"project manager|project management|\bpmo\b|programme manager|program manager|"
        r"project director|project lead|"
        r"(?:project|programme|program|information technology|software) delivery",
        "project management: planning and delivering projects on time and budget",
    ),
    (
        "general management",
        r"general manager|managing director|chief executive|country manager|\bceo\b",
        "general management: running a business or a country unit",
    ),
]
FAMILY_RES = [(name, re.compile(rx), words) for name, rx, words in FAMILIES]
FAMILY_WORDS = {name: words for name, _, words in FAMILIES}

# Titles a recruiter cannot place without knowing what the person did.
AMBIGUOUS = re.compile(
    r"^(senior |junior |principal |associate |lead |assistant |deputy )*"
    r"(consultant|analyst|officer|executive|"
    r"associate|specialist|engineer|manager|assistant|partner|business partner|account executive|qa engineer|"
    r"quality assurance engineer|director|coordinator|lead|head)$"
)

# What the work says (each sign strong on its own); the family with most
# signs wins, and a tie says nothing.
WORK_SIGNS = {
    "recruitment": r"placements?\b|placing|headhunt|360 desk|executive search|recruit|hired \d+|hiring for|"
    r"sourc\w* (candidates|engineers|talent)|shortlists? (to|for)|job fairs|interviews and offers|"
    r"(hired|recruited|placed) \w* ?(engineers|accountants|people|staff|candidates)|who hired",
    # A sales pipeline, not an oil, gas or data one; prospects who buy, not oil prospects.
    "sales": r"\bquota\b|new business|\bsold\b|\bsells?\b|selling|cold call|upsell|renewals|"
    r"closed .{0,20}(deals|business|sales)|close (deals|sales)|sales targets?|monthly targets|"
    r"(sales|deal|deals|lead|leads|client|customer|revenue|crm)( \w+){0,2} pipeline|"
    r"pipeline (of|in|on|with) .{0,25}"
    r"(deals|leads|clients|customers|opportunities|hubspot|salesforce|crm|rm ?\d)|"
    r"\bprospecting\b|\b(new|potential|qualified|sales) prospects?\b|\bprospects? (a|per|each|every) "
    r"(day|week|month)|\bprospective (clients?|customers?|buyers?)",
    "accounting": r"full set of accounts|bank reconciliation|month-end|ledger|accruals|management accounts|"
    r"three-way match|invoices|debt collection|credit control|statutory audit",
    "software testing": r"selenium|cypress|test cases|regression testing|test plans?|automated tests|defect",
    "manufacturing quality": r"haccp|\bgmp\b|iso 9001|\bcapa\b|incoming inspection|halal audit|"
    r"food (factory|factories|manufacturing|processing|production)",
    "software": r"\bjava\b|python|golang|\bgo\b and|react|node\.?js|microservices|kubernetes|\bapis?\b",
    "civil engineering": r"construction|contractors|earthworks|drainage|site supervision|mixed development",
    "oil and gas": r"\boffshore\b|subsea|drilling|\bwells\b|well (planning|testing|intervention|integrity)|"
    r"wellhead|reservoir|upstream|petroleum|\bfpso\b|(oil|gas|subsea|offshore) pipelines?|"
    r"pipeline (integrity|design|installation|inspection|engineering)|platforms? (maintenance|operations)",
    "procurement": r"purchase orders?|vendor quotations|suppliers? (evaluation|tenders)|price negotiation",
    "hr": r"employee relations|onboarding|hr policies|performance management|org design",
}
WORK_RES = {name: re.compile(rx, re.IGNORECASE) for name, rx in WORK_SIGNS.items()}

# Families a recruiter also looks at for each other (both ways): a payroll
# executive for an accounts role, an HR generalist for talent acquisition.
# A candidate whose best-matching title is in another family, not one of
# these, counts for less (the Go meaning module): an Account Executive who
# sells is not an Accounts Executive, an ERP sales director not an SAP
# consultant, however close their words.
_NEAR = {
    "recruitment": "hr, hr business partner",
    "hr": "hr business partner, payroll, reward, learning, administration",
    "hr business partner": "reward, learning",
    "payroll": "reward, accounting",
    "accounting": "administration",  # "Finance & Admin Executive" keeps the books
    "learning": "education",
    "erp finance": "erp",
    "erp": "business analysis, software",
    "finance management": "accounting, audit, treasury, tax, investment, risk",
    "audit": "accounting, tax, risk, compliance",
    "tax": "accounting",
    "treasury": "accounting, banking",
    "investment": "banking",
    "banking credit": "banking, risk",
    "banking": "sales",
    "risk": "compliance, insurance",
    "compliance": "legal, corporate secretarial",
    "customer success": "sales, customer service",
    "sales": "marketing, retail",
    "customer service": "retail, hospitality",
    "retail": "hospitality",
    "software testing": "software",
    "cyber security": "it infrastructure, cloud and devops, software, it support, compliance, risk",
    "operations": "logistics, supply chain planning, procurement, project management, general management, "
    "customer service, administration, sales, it support",
    "data": "software, business analysis, cloud and devops",
    "cloud and devops": "software, it infrastructure",
    "it infrastructure": "it support",
    "product management": "business analysis, project management, design",
    "business analysis": "project management",
    "design": "marketing, digital marketing",
    "quantity surveying": "civil engineering, building services",
    "building services": "civil engineering, mechanical engineering, electrical engineering",
    "oil and gas": "mechanical engineering, health and safety",
    "plantation": "mechanical engineering, manufacturing engineering",  # a palm oil mill's engineers
    "telecommunications": "it infrastructure, electrical engineering",
    "health and safety": "civil engineering, manufacturing engineering",
    "mechanical engineering": "manufacturing engineering, electrical engineering",
    "electrical engineering": "manufacturing engineering",
    "manufacturing engineering": "manufacturing quality",
    "procurement": "supply chain planning, logistics",
    "supply chain planning": "logistics",
    "digital marketing": "marketing",
    "corporate secretarial": "legal",
    "executive assistant": "administration",
}
NEAR: dict[str, frozenset[str]] = {}
for _a, _bs in _NEAR.items():
    for _b in (x.strip() for x in _bs.split(",")):
        NEAR[_a] = NEAR.get(_a, frozenset()) | {_b}
        NEAR[_b] = NEAR.get(_b, frozenset()) | {_a}
