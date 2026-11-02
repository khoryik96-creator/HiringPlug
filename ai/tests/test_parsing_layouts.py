"""CV layouts seen in real recruiting, one test each. Every name, number and
employer here is invented; the layouts copy real CVs that were parsed wrongly
before the rule under test existed."""

from __future__ import annotations

from hiringplug_ai.parsing import parse_cv

from .test_parsing import assert_grounded, jobs, v


def current(text: str) -> tuple[str | None, str | None]:
    p = parse_cv(text)
    job = next((j for j in p.experience if j.current), p.experience[0] if p.experience else None)
    assert job, "no job found"
    return v(job.title), v(job.company)


JOBSTREET = """AMIR HAKIM BIN OMAR
amir.hakim@example.com
+60 0120000001
Summary Seasoned IT professional with over 20 years of experience in
enterprise infrastructure.
Career history Senior Resident Engineer at Contoh Health
Feb 2025 - Present
Responsible for smooth operations of the Data Center.
Created with Jobstreet Profile
Senior Advisor - Global Storage and Backup at Dell
Technologies
Sep 2021 - Nov 2023 (2 years 3 months)
Attached to the Global Storage and Backup Support team.
Skills Microsoft Active Directory, VMware Administration, Storage Area
Network, ServiceNow
Languages English, Malay Language
Education Bachelor's Degree in Computer Science from University of
Tenaga Nasional
Finished 2001
Created with Jobstreet Profile
"""


def test_jobstreet_profile_export() -> None:
    p = parse_cv(JOBSTREET)
    assert v(p.fullName) == "Amir Hakim Bin Omar"
    assert jobs(p) == [
        ("Senior Resident Engineer", "Contoh Health", "2025-02", None, True),
        ("Senior Advisor - Global Storage and Backup", "Dell Technologies", "2021-09", "2023-11", False),
    ]
    assert [s.value for s in p.skills][:2] == ["Microsoft Active Directory", "VMware Administration"]
    assert "English" not in [s.value for s in p.skills]
    assert (v(p.education[0].institution), p.education[0].endYear) == ("University of Tenaga Nasional", 2001)
    assert_grounded(JOBSTREET, p)


def test_sidebar_timeline_with_dates_first_and_the_name_found_from_the_email() -> None:
    text = """CONTACT DETAILS
Mobile :
+6019-000 0001
Email :
meiling88lim@example.com
CAREER TIMELINE
Apr 2025 - Present
Head of Business Development
Contoh Healthcare Sdn Bhd
Feb 2024 - Mar 2025
National Key Account Manager
Contoh Hygiene Products (M) Sdn Bhd
CURRICULUM VITAE
LIM MEI LING
"""
    p = parse_cv(text)
    assert v(p.fullName) == "Lim Mei Ling"  # not "Career Timeline": the email names her
    assert v(p.phone) == "+6019-000 0001"  # the label is on the line above
    assert jobs(p) == [
        ("Head of Business Development", "Contoh Healthcare Sdn Bhd", "2025-04", None, True),
        ("National Key Account Manager", "Contoh Hygiene Products (M) Sdn Bhd", "2024-02", "2025-03", False),
    ]


def test_job_line_shapes() -> None:
    # Title above "Company | dates"; the company has no company word.
    assert current(
        "EXPERIENCE\nIT Engineer - Storage and Backup DevOps Engineer\nContohMobil IT | Jan 2025 - Present\n"
    ) == (
        "IT Engineer - Storage and Backup DevOps Engineer",
        "ContohMobil IT",
    )
    # "Company - Title (dates)": the whole rest of the line is the title.
    assert current(
        "EXPERIENCE\nDell Technologies - Senior Advisor - Database Administrator (2012 - Present)\n"
    ) == (
        "Senior Advisor - Database Administrator",
        "Dell Technologies",
    )
    # "Title, Department | Company, City | dates": the city is not part of the company.
    assert current(
        "EXPERIENCE\nSenior Consultant, Treasury & Markets Technology | Contoh Bank, Kuala Lumpur | Feb 2024 – Present\n"
    ) == ("Senior Consultant, Treasury & Markets Technology", "Contoh Bank")
    # "Employer client: Client | Title", dates on the next line with a place after them.
    assert current(
        "EXPERIENCE\nCoforge client :Contoh Bank Malasiya| Senior DBA (Oracle/MSSQL)\nJuly- 2026 – Present KL Malasiya\n"
    ) == ("Senior DBA (Oracle/MSSQL)", "Coforge")
    # Labelled lines, and year-first dates without a dash between them.
    assert current(
        "PROFESSIONAL EXPERIENCE\nCompany: Beemacs\nProject Name: Contoh\nRole: Senior SAP TM technical consultant.\n"
        "Duration: (2022-Oct 2023-May)\n"
    ) == ("Senior SAP TM technical consultant", "Beemacs")
    # Numbered employers with the title below.
    assert current(
        "PROFESSIONAL EXPERIENCE\n1. Contoh Finance House Jun 2008 - present\nIT Application Manager\n"
    ) == (
        "IT Application Manager",
        "Contoh Finance House",
    )
    # A title followed by a duty, and a date range broken over two lines.
    assert current(
        "WORK EXPERIENCE\nHead of Sales - Lead and develop the key account team\nContoh Sdn Bhd | May 2020 –\nPresent\n"
    ) == (
        "Head of Sales",
        "Contoh Sdn Bhd",
    )
    # LinkedIn exports: company, title, then dates with a duration.
    assert current(
        "Experience\nContoh Bank Berhad\nSenior Manager, Market Risk\nApril 2026 - Present (7 months)\n"
    ) == (
        "Senior Manager, Market Risk",
        "Contoh Bank Berhad",
    )


def test_name_shapes() -> None:
    def name(text: str) -> str | None:
        return v(parse_cv(text).fullName)

    assert name("TAN WEI LING Senior Oracle Database Engineer | Oracle DBA\n") == "Tan Wei Ling"
    assert name("LIM AH KOW (JORDAN) Curriculum Vitae\nNationality: Malaysian\n") == "Lim Ah Kow"
    assert name("Ts. Arvind Kumaran, PMP®\nSenior Database Administrator\n") == "Arvind Kumaran"
    assert name("Ong Wei Jie (Aaron)\nCommercial Leader | Partnerships\n") == "Ong Wei Jie"
    assert name("Email ID: naresh.k@example.com\nNaresh K Phone: +91 9000000001\n") == "Naresh K"
    assert (
        name("KUMARAN\nEngineering Lead | SRE\nlinkedin.com/in/kumaran-p\n") == "Kumaran"
    )  # one word, but linked
    assert name("Dynamic and result-oriented professional with 15 years in banking.\n") is None
    assert name("CONTACT\nSeri Kembangan,\nMalaysia, 43300\n") is None  # addresses are not names


def test_headings_and_skills() -> None:
    text = """C ON TA CT
aina@example.com
S KILL S
Cloud Platforms: AWS (EC2, EKS,
S3), Azure
Linux, Solaris
Very Good
lllllllllllllllllllll
Installed, configured and supported Oracle databases for enterprise customers.
"""
    p = parse_cv(text)
    assert [s.value for s in p.skills] == ["AWS (EC2, EKS, S3)", "Azure", "Linux", "Solaris"]
    assert_grounded(text, p)
    # A wrapped "... at Dell" + "Technologies" is not a skills heading.
    p = parse_cv("EXPERIENCE\nSenior Advisor at Dell\nTechnologies\nSep 2021 - Nov 2023\nSKILLS\nExcel\n")
    assert [s.value for s in p.skills] == ["Excel"]
