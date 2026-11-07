from __future__ import annotations

import pytest

from hiringplug_ai.config import ConfigError, load_config


def test_defaults() -> None:
    config = load_config({"HP_AI_DATABASE_URL": "postgresql://hp_ai:x@localhost/hiringplug"})
    assert config.ocr_enabled
    assert config.ocr_languages == "eng"
    assert config.lease_seconds == 600
    assert config.worker_id.startswith("py:")
    assert config.concurrency == 3, "a slow AI call does not hold up reading a new CV"


def test_concurrency_is_bounded() -> None:
    with pytest.raises(ConfigError, match="HP_AI_CONCURRENCY"):
        load_config({"HP_AI_DATABASE_URL": "postgresql://x@localhost/h", "HP_AI_CONCURRENCY": "0"})


def test_every_problem_is_reported_at_once() -> None:
    with pytest.raises(ConfigError) as err:
        load_config({"HP_AI_OCR": "maybe", "HP_AI_POLL_SECONDS": "soon", "HP_LOG_LEVEL": "loud"})
    for key in ("HP_AI_DATABASE_URL", "HP_AI_OCR", "HP_AI_POLL_SECONDS", "HP_LOG_LEVEL"):
        assert key in str(err.value)
