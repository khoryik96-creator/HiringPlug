"""Reject zip bombs before opening Office documents (DOCX files are zip archives)."""

from __future__ import annotations

import io
import zipfile

from .limits import DocumentRejectedError, Limits


def validate_zip(data: bytes, limits: Limits, label: str = "document") -> None:
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            entries = archive.infolist()
    except zipfile.BadZipFile as exc:
        raise DocumentRejectedError(f"The {label} is damaged and cannot be opened.") from exc
    if len(entries) > limits.max_zip_entries:
        raise DocumentRejectedError(f"The {label} contains too many internal files.")
    total = 0
    for entry in entries:
        size = max(0, entry.file_size)
        ratio = size / max(1, entry.compress_size)
        total += size
        if size > limits.max_zip_single_entry_bytes:
            raise DocumentRejectedError(f"The {label} contains an oversized internal file.")
        if size > 8 * 1024 * 1024 and ratio > 1000:
            raise DocumentRejectedError(f"The {label} has an unsafe compression ratio.")
        if total > limits.max_zip_expanded_bytes:
            raise DocumentRejectedError(f"The {label} expands beyond the safe size limit.")
