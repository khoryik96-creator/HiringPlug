"""The meaning model: all-MiniLM-L6-v2 (Apache 2.0), as ONNX, on the CPU.

It turns a text into 384 numbers; texts that mean the same thing get close
numbers, whatever their words. Compared with larger models on a recruiter
bench (titles, CVs and searches, with traps such as "Account Executive" and
"Accounts Executive") it did as well, and it reads about twice as fast, which
matters when it first reads every CV on an ordinary computer.
"""

from __future__ import annotations

import os
import threading
from collections.abc import Sequence
from pathlib import Path

import numpy as np
import onnxruntime as ort  # type: ignore[import-untyped]
from tokenizers import Tokenizer

# The reading rules' version (recruiter.py, families.py, passages.py): raise it
# when they change, and every candidate is read again with the new rules.
READING = 4
# Reported with every result: vectors of another model (or other rules) are
# never compared; a new name makes the app read everyone again.
NAME = f"all-MiniLM-L6-v2@1110a243/r{READING}"
DIMENSIONS = 384
MAX_TOKENS = 256  # the model's own limit; longer texts are cut
BATCH = 32


def default_folder(env: dict[str, str] | None = None) -> Path:
    """HP_AI_MEANING_MODEL_DIR, else ai/.models/all-MiniLM-L6-v2 in a checkout
    (where `task setup` puts it)."""
    env = dict(os.environ) if env is None else env
    chosen = env.get("HP_AI_MEANING_MODEL_DIR", "").strip()
    if chosen:
        return Path(chosen)
    return Path(__file__).resolve().parents[3] / ".models" / "all-MiniLM-L6-v2"


class MeaningModel:
    """Thread-safe: the worker's slots share one."""

    def __init__(self, folder: Path) -> None:
        self._tokenizer = Tokenizer.from_file(str(folder / "tokenizer.json"))
        self._tokenizer.enable_truncation(MAX_TOKENS)
        self._tokenizer.enable_padding()
        options = ort.SessionOptions()
        options.intra_op_num_threads = max(1, min(4, os.cpu_count() or 1))
        self._session = ort.InferenceSession(
            str(folder / "model.onnx"), options, providers=["CPUExecutionProvider"]
        )
        self._inputs = {i.name for i in self._session.get_inputs()}
        self._lock = threading.Lock()  # the tokenizer's padding settings are shared state

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        """One unit-length row of 384 numbers per text (mean of the tokens)."""
        if not texts:
            return np.zeros((0, DIMENSIONS), dtype=np.float32)
        out = []
        for start in range(0, len(texts), BATCH):
            with self._lock:
                encoded = self._tokenizer.encode_batch([t or " " for t in texts[start : start + BATCH]])
            ids = np.array([e.ids for e in encoded], dtype=np.int64)
            mask = np.array([e.attention_mask for e in encoded], dtype=np.int64)
            feed = {"input_ids": ids, "attention_mask": mask}
            if "token_type_ids" in self._inputs:
                feed["token_type_ids"] = np.zeros_like(ids)
            hidden = self._session.run(None, feed)[0]
            weights = mask[..., None].astype(np.float32)
            mean = (hidden * weights).sum(axis=1) / np.maximum(weights.sum(axis=1), 1e-9)
            norms = np.linalg.norm(mean, axis=1, keepdims=True)
            out.append(mean / np.maximum(norms, 1e-9))
        return np.vstack(out).astype(np.float32)


_loaded: dict[Path, MeaningModel] = {}
_loading = threading.Lock()


def installed(folder: Path) -> bool:
    return (folder / "model.onnx").is_file() and (folder / "tokenizer.json").is_file()


def load(folder: Path | None = None) -> MeaningModel | None:
    """The model in folder (loaded once), or None when it is not installed."""
    folder = folder or default_folder()
    with _loading:
        if folder not in _loaded:
            if not installed(folder):
                return None
            _loaded[folder] = MeaningModel(folder)
        return _loaded[folder]


def as_list(vector: np.ndarray) -> list[float]:
    """A vector for a job message: rounded (five places keep its meaning) to keep messages small."""
    return [round(float(x), 5) for x in vector]
