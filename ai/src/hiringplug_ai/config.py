"""Settings, read once from HP_AI_* environment variables (see .env.example).

Every problem is reported at once, and the worker refuses to start with any.
"""

from __future__ import annotations

import os
import socket
from collections.abc import Mapping
from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    # The Python worker's own database role: it may only call the job queue
    # functions, never read candidate tables.
    database_url: str
    worker_id: str
    log_level: str
    poll_seconds: float
    lease_seconds: int
    ocr_enabled: bool
    ocr_languages: str  # e.g. "eng", or "eng+msa" for English and Malay
    max_download_bytes: int


class ConfigError(ValueError):
    pass


def load_config(env: Mapping[str, str]) -> Config:
    problems: list[str] = []

    def number(key: str, default: float) -> float:
        raw = env.get(key, "").strip()
        if not raw:
            return default
        try:
            return float(raw)
        except ValueError:
            problems.append(f"{key} must be a number, got {raw!r}")
            return default

    database_url = env.get("HP_AI_DATABASE_URL", "").strip()
    if not database_url:
        problems.append("HP_AI_DATABASE_URL is required (the AI worker's restricted database role)")
    level = env.get("HP_LOG_LEVEL", "info").strip().upper() or "INFO"
    if level not in {"DEBUG", "INFO", "WARN", "WARNING", "ERROR"}:
        problems.append(f"HP_LOG_LEVEL must be debug, info, warn or error, got {level.lower()!r}")
    ocr = env.get("HP_AI_OCR", "on").strip().lower()
    if ocr not in {"on", "off"}:
        problems.append(f"HP_AI_OCR must be on or off, got {ocr!r}")

    config = Config(
        database_url=database_url,
        worker_id=env.get("HP_AI_WORKER_ID", "").strip() or f"py:{socket.gethostname()}:{os.getpid()}",
        log_level="WARNING" if level == "WARN" else level,
        poll_seconds=number("HP_AI_POLL_SECONDS", 2.0),
        lease_seconds=int(number("HP_AI_LEASE_SECONDS", 600)),
        ocr_enabled=ocr == "on",
        ocr_languages=env.get("HP_AI_OCR_LANGUAGES", "eng").strip() or "eng",
        max_download_bytes=int(number("HP_MAX_UPLOAD_BYTES", 10 * 1024 * 1024)),
    )
    if config.lease_seconds < 30:
        problems.append("HP_AI_LEASE_SECONDS must be at least 30")
    if problems:
        raise ConfigError("invalid configuration:\n  - " + "\n  - ".join(problems))
    return config
