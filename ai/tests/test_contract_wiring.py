"""The Python worker must agree with the contract: it handles every job on the
`ai` queue, and sends results to the queue the contract names."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml

from hiringplug_ai import contracts
from hiringplug_ai.jobs import HANDLERS, extract_text

CONTRACT = Path(__file__).resolve().parents[2] / "contracts" / "openapi.yaml"


def x_jobs() -> dict[str, Any]:
    spec = yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))
    jobs: dict[str, Any] = spec["x-jobs"]
    return jobs


def test_every_ai_queue_job_has_a_python_handler() -> None:
    ai_kinds = {kind for kind, info in x_jobs().items() if info["queue"] == "ai"}
    handled = {kind.value for kind in HANDLERS}
    assert ai_kinds == handled, "add a handler in hiringplug_ai/jobs (or move the job to the core queue)"


def test_handlers_use_the_payload_schema_the_contract_names() -> None:
    jobs = x_jobs()
    for kind, handler in HANDLERS.items():
        assert handler.payload_model.__name__ == jobs[kind.value]["payload"]


def test_results_go_to_the_queue_the_contract_names() -> None:
    assert x_jobs()["document.text_extracted"]["queue"] == extract_text.RESULT_QUEUE


def test_generated_models_cover_every_job_payload() -> None:
    for info in x_jobs().values():
        assert hasattr(contracts, info["payload"]), "run `task gen` to regenerate hiringplug_ai/contracts.py"
