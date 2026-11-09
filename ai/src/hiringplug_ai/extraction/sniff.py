"""Identify a file from its bytes; the name and claimed type are not trusted."""

from __future__ import annotations

import io
import zipfile
from typing import Literal

Kind = Literal["pdf", "docx", "doc", "image", "unknown"]

# Old Word files are OLE "compound documents" (so are old Excel and PowerPoint
# files; the .doc reader refuses those).
OLE_MAGIC = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"


# Pictures of a CV (read with OCR): PNG, JPEG, TIFF, BMP, WEBP.
_IMAGE_MAGIC = (b"\x89PNG\r\n\x1a\n", b"\xff\xd8\xff", b"II*\x00", b"MM\x00*", b"BM")


def sniff(data: bytes) -> Kind:
    if data.startswith(b"%PDF-"):
        return "pdf"
    if data.startswith(_IMAGE_MAGIC) or (data[:4] == b"RIFF" and data[8:12] == b"WEBP"):
        return "image"
    if data.startswith(OLE_MAGIC):
        return "doc"
    if data.startswith(b"PK\x03\x04"):
        try:
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                if "word/document.xml" in archive.namelist():
                    return "docx"
        except zipfile.BadZipFile:
            return "unknown"
    return "unknown"
