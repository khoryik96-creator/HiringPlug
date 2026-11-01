"""The AI worker: claims jobs from the `ai` queue and runs their handlers.

    uv run hiringplug-ai        (settings: HP_AI_* environment variables)

Each job is processed with its job_id, kind, tenant_id and the original
request_id on every log line. A handler's follow-up jobs and the job's
completion are committed together; any exception records a failure, and the
queue retries the job later (or marks it dead after its last attempt).
"""

from __future__ import annotations

import logging
import os
import signal
import sys
import threading
import time

import psycopg

from . import logs
from .config import ConfigError, load_config
from .contracts import JobKind
from .download import download
from .extraction import DEFAULT_LIMITS, TesseractOcr
from .jobs import HANDLERS, Deps
from .queue import ClaimedJob, LeaseLostError, Queue

log = logging.getLogger("hiringplug_ai.worker")


def process(queue: Queue, job: ClaimedJob, deps: Deps) -> None:
    with logs.context(
        job_id=job.id,
        job_kind=job.kind,
        tenant_id=job.tenant_id,
        request_id=job.request_id,
        attempt=job.attempts,
    ):
        started = time.monotonic()
        try:
            handler = HANDLERS[JobKind(job.kind)]
            payload = handler.payload_model.model_validate(job.payload)
            queue.complete(job, handler.run(payload, deps))
            log.info("job done", extra={"duration_ms": int((time.monotonic() - started) * 1000)})
        except LeaseLostError:
            log.warning("job outlived its lease; its results were discarded and another worker owns it")
        except Exception as exc:  # every failure must be recorded on the job
            status = queue.fail(job, f"{type(exc).__name__}: {exc}")
            log.exception("job failed", extra={"new_status": status, "max_attempts": job.max_attempts})


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
    deps = Deps(
        download=download, ocr=ocr, limits=DEFAULT_LIMITS, max_download_bytes=config.max_download_bytes
    )
    queue = Queue(config.database_url, config.worker_id)

    stop = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.set())

    log.info("worker started", extra={"worker_id": config.worker_id, "ocr": ocr.name if ocr else "off"})
    while not stop.is_set():
        try:
            job = queue.claim(config.lease_seconds)
            if job is None:
                queue.wait_for_work(config.poll_seconds)
                continue
            process(queue, job, deps)
        except psycopg.OperationalError:
            log.exception("database unavailable; retrying shortly")
            queue.close()
            stop.wait(config.poll_seconds)
    queue.close()
    log.info("worker stopped")


if __name__ == "__main__":
    main()
