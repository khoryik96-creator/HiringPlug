"""Downloads the meaning model, pinned and checked:

    uv run python -m hiringplug_ai.meaning.fetch [folder]

The files come from Hugging Face at a fixed revision and are kept only when
their SHA-256 is the one below, so the model cannot change under the app.
Run by `task setup` and when the AI worker's image is built.
"""

from __future__ import annotations

import hashlib
import sys
from pathlib import Path

import httpx

from .folder import default_folder

REPO = "sentence-transformers/all-MiniLM-L6-v2"
REVISION = "1110a243fdf4706b3f48f1d95db1a4f5529b4d41"
FILES = {
    "model.onnx": ("onnx/model.onnx", "6fd5d72fe4589f189f8ebc006442dbb529bb7ce38f8082112682524616046452"),
    "tokenizer.json": ("tokenizer.json", "be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037"),
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def fetch(folder: Path) -> list[str]:
    """Downloads what is missing or wrong; returns the files fetched."""
    folder.mkdir(parents=True, exist_ok=True)
    fetched = []
    for name, (remote, want) in FILES.items():
        target = folder / name
        if target.is_file() and sha256(target) == want:
            continue
        url = f"https://huggingface.co/{REPO}/resolve/{REVISION}/{remote}"
        partial = target.with_suffix(target.suffix + ".part")
        with httpx.stream("GET", url, follow_redirects=True, timeout=120) as response:
            response.raise_for_status()
            with partial.open("wb") as out:
                for chunk in response.iter_bytes(1 << 20):
                    out.write(chunk)
        got = sha256(partial)
        if got != want:
            partial.unlink(missing_ok=True)
            raise RuntimeError(f"{name}: SHA-256 {got} is not the pinned {want}; not kept")
        partial.replace(target)
        fetched.append(name)
    return fetched


def main() -> None:
    folder = Path(sys.argv[1]) if len(sys.argv) > 1 else default_folder()
    fetched = fetch(folder)
    print(f"meaning model in {folder}: " + (f"fetched {', '.join(fetched)}" if fetched else "already there"))


if __name__ == "__main__":
    main()
