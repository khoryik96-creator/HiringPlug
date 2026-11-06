"""contact.read_linkedin: a contact's LinkedIn profile PDF, read by the rules."""

from __future__ import annotations

from hiringplug_ai.contracts import ContactLinkedinReadJob, ContactReadLinkedinJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, read_linkedin
from tests.documents import text_pdf

CHECK = "cec_01jbz6w0m0k8q9x2r3s4t5v6w7"
PROFILE = [
    "Contact",
    "www.linkedin.com/in/nur-aina-example (LinkedIn)",
    "Nur Aina Example",
    "Talent Acquisition Lead at Contoh Bank Berhad",
    "Kuala Lumpur, Malaysia",
    "Experience",
    "Contoh Bank Berhad",
    "Talent Acquisition Lead",
    "April 2026 - Present (7 months)",
    "Maju Sdn Bhd",
    "HR Manager",
    "March 2021 - March 2026 (5 years 1 month)",
]


def run(data: bytes) -> ContactLinkedinReadJob:
    deps = Deps(download=lambda url, n: data, ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=10_000_000)
    job = ContactReadLinkedinJob(
        checkId=CHECK, downloadUrl="https://files/x", fileName="Profile.pdf", sizeBytes=len(data)
    )
    [f] = read_linkedin.run(job, deps)
    assert f.queue == "core"
    assert f.kind == JobKind.contact_linkedin_read
    return ContactLinkedinReadJob.model_validate(f.payload.model_dump(mode="json"))


def test_reads_the_jobs_on_a_profile() -> None:
    r = run(text_pdf(PROFILE))
    assert r.errorMessage is None
    assert r.profile is not None
    jobs = [
        (j.title and j.title.value, j.company and j.company.value, j.current) for j in r.profile.experience
    ]
    assert jobs[0] == ("Talent Acquisition Lead", "Contoh Bank Berhad", True)
    assert ("HR Manager", "Maju Sdn Bhd", False) in jobs
    assert r.profile.linkedinUrl is not None
    assert "nur-aina-example" in r.profile.linkedinUrl.value


def test_a_file_without_text_says_so() -> None:
    r = run(b"%PDF-1.4\n%%EOF")
    assert r.profile is None
    assert r.errorMessage
