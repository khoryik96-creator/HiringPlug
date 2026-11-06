"""Settings, read once from HP_AI_* environment variables (see .env.example).

Every problem is reported at once, and the worker refuses to start with any.
"""

from __future__ import annotations

import os
import socket
from collections.abc import Mapping
from dataclasses import dataclass, field


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
    # The AI provider: "off", or "openai_compatible" (DeepSeek's API, a host
    # running its open model, or your own server).
    ai_provider: str = "off"
    ai_url: str = ""
    ai_key: str = field(default="", repr=False)  # never printed
    ai_model: str = ""
    ai_timeout_seconds: float = 90.0
    # Opens the API keys companies save in Settings (the API seals them with
    # the same secret). Empty: those keys cannot be used.
    ai_keys_secret: str = field(default="", repr=False)  # never printed


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

    provider = env.get("HP_AI_PROVIDER", "off").strip().lower() or "off"
    if provider not in {"off", "openai_compatible"}:
        problems.append(f"HP_AI_PROVIDER must be off or openai_compatible, got {provider!r}")
    ai_url = env.get("HP_AI_PROVIDER_URL", "").strip()
    ai_model = env.get("HP_AI_MODEL", "").strip()
    if provider == "openai_compatible":
        if not ai_url.startswith(("https://", "http://")):
            problems.append(
                "HP_AI_PROVIDER_URL must be the provider's address, e.g. https://api.deepseek.com/v1"
            )
        if not ai_model:
            problems.append("HP_AI_MODEL is required with an AI provider, e.g. deepseek-chat")

    config = Config(
        database_url=database_url,
        worker_id=env.get("HP_AI_WORKER_ID", "").strip() or f"py:{socket.gethostname()}:{os.getpid()}",
        log_level="WARNING" if level == "WARN" else level,
        poll_seconds=number("HP_AI_POLL_SECONDS", 2.0),
        lease_seconds=int(number("HP_AI_LEASE_SECONDS", 600)),
        ocr_enabled=ocr == "on",
        ocr_languages=env.get("HP_AI_OCR_LANGUAGES", "eng").strip() or "eng",
        max_download_bytes=int(number("HP_MAX_UPLOAD_BYTES", 10 * 1024 * 1024)),
        ai_provider=provider,
        ai_url=ai_url,
        ai_key=env.get("HP_AI_PROVIDER_KEY", "").strip(),
        ai_model=ai_model,
        ai_timeout_seconds=number("HP_AI_TIMEOUT_SECONDS", 90.0),
        ai_keys_secret=env.get("HP_AI_KEYS_SECRET", "").strip(),
    )
    if 0 < len(config.ai_keys_secret) < 32:
        problems.append("HP_AI_KEYS_SECRET must be at least 32 characters (or empty)")
    if config.ai_timeout_seconds <= 0 or config.ai_timeout_seconds * 2 >= config.lease_seconds:
        problems.append("HP_AI_TIMEOUT_SECONDS must be positive and well under HP_AI_LEASE_SECONDS")
    if config.lease_seconds < 30:
        problems.append("HP_AI_LEASE_SECONDS must be at least 30")
    if problems:
        raise ConfigError("invalid configuration:\n  - " + "\n  - ".join(problems))
    return config
