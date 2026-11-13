"""The AI worker: claims jobs from the `ai` queue and runs their handlers.

    uv run hiringplug-ai        (settings: HP_AI_* environment variables)

Each job is processed with its job_id, kind, tenant_id and the original
request_id on every log line. A handler's follow-up jobs and the job's
completion are committed together; any exception records a failure, and the
queue retries the job later (or marks it dead after its last attempt).
"""

from __future__ import annotations

import dataclasses
import logging
import os
import signal
import sys
import threading
import time

import psycopg

from . import logs
from .config import Config, ConfigError, load_config
from .contracts import JobKind
from .download import download
from .extraction import DEFAULT_LIMITS, TesseractOcr
from .jobs import HANDLERS, Deps, ProviderLog
from .providers import OpenAICompatible, connect
from .queue import ClaimedJob, LeaseLostError, Queue

log = logging.getLogger("hiringplug_ai.worker")


def process(queue: Queue, job: ClaimedJob, deps: Deps, lease_seconds: int | None = None) -> None:
    with logs.context(
        job_id=job.id,
        job_kind=job.kind,
        tenant_id=job.tenant_id,
        request_id=job.request_id,
        attempt=job.attempts,
    ):
        started = time.monotonic()
        calls = ProviderLog()  # what the AI provider did, told to the app with the result
        try:
            handler = HANDLERS[JobKind(job.kind)]
            payload = handler.payload_model.model_validate(job.payload)
            deps = dataclasses.replace(deps, log=calls)
            if lease_seconds:
                deps = dataclasses.replace(deps, keep_alive=lambda: queue.extend(job, lease_seconds))
            follow_ups = handler.run(payload, deps)
            report = calls.report()
            queue.complete(job, [*follow_ups, report] if report else follow_ups)
            log.info("job done", extra={"duration_ms": int((time.monotonic() - started) * 1000)})
        except LeaseLostError:
            log.warning("job outlived its lease; its results were discarded and another worker owns it")
        except Exception as exc:  # every failure must be recorded on the job
            status = queue.fail(job, f"{type(exc).__name__}: {exc}")
            log.exception("job failed", extra={"new_status": status, "max_attempts": job.max_attempts})
            report = calls.report()
            # The provider is why: say so now, not after the retries.
            if report is not None and calls.ok is False:
                try:
                    queue.enqueue(job, report)
                except psycopg.Error:  # telling is best effort; the failure itself is recorded
                    log.warning("could not report the AI provider's failure")


def main() -> None:
    try:
        config = load_config(os.environ)
    except ConfigError as exc:
        print(f"ai-worker: {exc}", file=sys.stderr)  # logging is not configured yet
        sys.exit(1)
    logs.configure(config.log_level)

    ocr = TesseractOcr(config.ocr_languages) if config.ocr_enabled else None
    if ocr is not None and not ocr.available():
        log.warning("OCR is on but Tesseract is not installed; scanned documents will be marked needs_ocr")
    ai = (
        OpenAICompatible(config.ai_url, config.ai_key, config.ai_model, config.ai_timeout_seconds)
        if config.ai_provider == "openai_compatible"
        else None
    )
    deps = Deps(
        download=download,
        ocr=ocr,
        limits=DEFAULT_LIMITS,
        max_download_bytes=config.max_download_bytes,
        ai=ai,
        long_timeout_seconds=config.ai_long_timeout_seconds,
        connect=(
            (lambda access: connect(access, config.ai_keys_secret, config.ai_timeout_seconds))
            if config.ai_keys_secret
            else None
        ),
    )
    stop = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.set())

    log.info(
        "worker started",
        extra={
            "worker_id": config.worker_id,
            "ocr": ocr.name if ocr else "off",
            "ai": config.ai_model or "off",
            "company_keys": "on" if config.ai_keys_secret else "off",
            "concurrency": config.concurrency,
        },
    )
    queues = [
        Queue(config.database_url, slot_id(config.worker_id, i, config.concurrency))
        for i in range(config.concurrency)
    ]
    slots = [
        threading.Thread(target=run_slot, args=(q, deps, config, stop), name=f"slot-{i + 1}")
        for i, q in enumerate(queues)
    ]
    for t in slots:
        t.start()
    while any(t.is_alive() for t in slots):
        for t in slots:
            t.join(timeout=0.5)  # wakes for signals
    log.info("worker stopped")


def slot_id(worker_id: str, i: int, n: int) -> str:
    """Each slot holds its own leases, so it needs its own worker ID."""
    return worker_id if n == 1 else f"{worker_id}/{i + 1}"


def run_slot(queue: Queue, deps: Deps, config: Config, stop: threading.Event) -> None:
    """Runs jobs one after another until stop is set (one of several slots)."""
    while not stop.is_set():
        try:
            job = queue.claim(config.lease_seconds)
            if job is None:
                queue.wait_for_work(config.poll_seconds)
                continue
            process(queue, job, deps, config.lease_seconds)
        except psycopg.OperationalError:
            log.exception("database unavailable; retrying shortly")
            queue.close()
            stop.wait(config.poll_seconds)
    queue.close()


if __name__ == "__main__":
    main()
