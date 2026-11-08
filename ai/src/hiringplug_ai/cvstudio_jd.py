"""CV Studio's JD blinding helpers (vendored unchanged in ai/cvstudio/jd:
its prompt and its local checks), run by Node through jd_cli.cjs, a small
adapter, as CV Studio's Word renderer is.
"""

from __future__ import annotations

import json
import subprocess
from typing import Any

from .cvstudio import ROOT

JD = ROOT / "jd"
TIMEOUT_SECONDS = 30


class GuardError(RuntimeError):
    """CV Studio's JD checks could not run (never carries the ad's text)."""


def _call(op: str, **request: Any) -> Any:
    done = subprocess.run(  # noqa: S603 - fixed program and script, our own files
        ["node", str(JD / "jd_cli.cjs")],  # noqa: S607 - node from PATH
        input=json.dumps({"op": op, **request}, ensure_ascii=False),
        capture_output=True,
        text=True,
        timeout=TIMEOUT_SECONDS,
        check=False,
    )
    try:
        answer = json.loads(done.stdout or "{}")
    except ValueError as exc:
        raise GuardError(f"CV Studio's JD checks ({op}) failed") from exc
    if not isinstance(answer, dict) or not answer.get("ok"):
        raise GuardError(f"CV Studio's JD checks ({op}) failed")
    return answer.get("value")


def normalize(raw: str) -> str:
    """The text with PDF ligature and spacing damage repaired, as CV Studio does first."""
    return str(_call("normalize", raw=raw))


def prompt(**options: str) -> str:
    """CV Studio's exact prompt for these options (raw, companyName, industry,
    location, workMode, clientDesc, context, redact, whyJoinUs, tone, strength)."""
    return str(_call("prompt", **options))


def issues(jd: dict[str, Any], raw: str) -> list[str]:
    """The sections still copying long runs of the original wording."""
    return [str(x) for x in _call("issues", jd=jd, raw=raw)]


def needs_retry(jd: dict[str, Any], raw: str) -> bool:
    return bool(_call("needsRetry", jd=jd, raw=raw))


def retry_instruction(found: list[str]) -> str:
    return str(_call("retryInstruction", issues=found))


def finalize(jd: dict[str, Any], raw: str, company_name: str) -> dict[str, Any]:
    """CV Studio's last steps, in its order: the source's genuine tech terms put
    back in the stack, company scale softened, the client's name scrubbed;
    with the terms missing before and after, and copied sections left."""
    value = _call("finalize", jd=jd, raw=raw, companyName=company_name)
    if not isinstance(value, dict):
        raise GuardError("CV Studio's JD checks (finalize) failed")
    return value
