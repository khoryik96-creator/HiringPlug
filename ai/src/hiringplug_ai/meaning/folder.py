"""Where the meaning model's files are. Kept apart from model.py so that
reading the settings, and fetching the model, never load the ONNX runtime."""

from __future__ import annotations

import os
from pathlib import Path


def default_folder(env: dict[str, str] | None = None) -> Path:
    """HP_AI_MEANING_MODEL_DIR, else ai/.models/all-MiniLM-L6-v2 in a checkout
    (where `task setup` puts it)."""
    env = dict(os.environ) if env is None else env
    chosen = env.get("HP_AI_MEANING_MODEL_DIR", "").strip()
    if chosen:
        return Path(chosen)
    return Path(__file__).resolve().parents[3] / ".models" / "all-MiniLM-L6-v2"
