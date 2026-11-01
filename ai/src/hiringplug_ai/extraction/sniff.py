"""Identify a file from its bytes; the name and claimed type are not trusted."""

from __future__ import annotations

import io
import zipfile
from typing import Literal

Kind = Literal["pdf", "docx", "unknown"]


def sniff(data: bytes) -> Kind:
    if data.startswith(b"%PDF-"):
        return "pdf"
    if data.startswith(b"PK\x03\x04"):
        try:
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                if "word/document.xml" in archive.namelist():
                    return "docx"
        except zipfile.BadZipFile:
            return "unknown"
    return "unknown"
