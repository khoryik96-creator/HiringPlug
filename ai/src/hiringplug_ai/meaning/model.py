"""The meaning model: all-MiniLM-L6-v2 (Apache 2.0), as ONNX, on the CPU.

It turns a text into 384 numbers; texts that mean the same thing get close
numbers, whatever their words. Compared with larger models on a recruiter
bench (titles, CVs and searches, with traps such as "Account Executive" and
"Accounts Executive") it did as well, and it reads about twice as fast, which
matters when it first reads every CV on an ordinary computer.
"""

from __future__ import annotations

import logging
import os
import threading
from collections.abc import Sequence
from pathlib import Path

import numpy as np

from .folder import default_folder

log = logging.getLogger(__name__)

# The reading rules' version (recruiter.py, families.py, passages.py): raise it
# when they change, and every candidate is read again with the new rules.
READING = 6
# Reported with every result: vectors of another model (or other rules) are
# never compared; a new name makes the app read everyone again.
NAME = f"all-MiniLM-L6-v2@1110a243/r{READING}"
DIMENSIONS = 384
MAX_TOKENS = 256  # the model's own limit; longer texts are cut
BATCH = 32


class MeaningModel:
    """Thread-safe: the worker's slots share one."""

    def __init__(self, folder: Path) -> None:
        # Loaded here, not when the module is imported: a worker on a computer
        # the ONNX runtime does not support still runs, without this search.
        import onnxruntime as ort  # type: ignore[import-untyped]  # noqa: PLC0415
        from tokenizers import Tokenizer  # noqa: PLC0415

        self._tokenizer = Tokenizer.from_file(str(folder / "tokenizer.json"))
        self._tokenizer.enable_truncation(MAX_TOKENS)
        self._tokenizer.enable_padding()
        options = ort.SessionOptions()
        # Half the cores at most: the background read-through must leave room
        # for reading CVs and for the database; no spinning while idle.
        options.intra_op_num_threads = max(1, min(4, (os.cpu_count() or 2) // 2))
        options.inter_op_num_threads = 1
        options.add_session_config_entry("session.intra_op.allow_spinning", "0")
        self._session = ort.InferenceSession(
            str(folder / "model.onnx"), options, providers=["CPUExecutionProvider"]
        )
        self._inputs = {i.name for i in self._session.get_inputs()}
        self._lock = threading.Lock()  # the tokenizer's padding settings are shared state

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        """One unit-length row of 384 numbers per text (mean of the tokens)."""
        if not texts:
            return np.zeros((0, DIMENSIONS), dtype=np.float32)
        # Read in batches of texts of about the same length (a batch is padded
        # to its longest), then put back in the order asked for.
        order = sorted(range(len(texts)), key=lambda i: len(texts[i]))
        ordered = [texts[i] for i in order]
        out = []
        for start in range(0, len(ordered), BATCH):
            with self._lock:
                encoded = self._tokenizer.encode_batch([t or " " for t in ordered[start : start + BATCH]])
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
        rows = np.vstack(out).astype(np.float32)
        result = np.empty_like(rows)
        result[order] = rows
        return result


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
            try:
                _loaded[folder] = MeaningModel(folder)
            except ImportError as e:
                log.warning("search by meaning is off: the ONNX runtime cannot be loaded here (%s)", e)
                return None
        return _loaded[folder]


def as_list(vector: np.ndarray) -> list[float]:
    """A vector for a job message: rounded (five places keep its meaning) to keep messages small."""
    return [round(float(x), 5) for x in vector]
