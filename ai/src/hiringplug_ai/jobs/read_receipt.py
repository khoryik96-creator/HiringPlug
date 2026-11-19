"""claim.read_receipt: read an expense claim's receipt (PDF text, or OCR for
photos and scans) and send what it says back to Go: its date, total,
currency, merchant and type, each a suggestion the claimant can change."""

from __future__ import annotations

import logging
from datetime import UTC, datetime
from typing import TYPE_CHECKING

from ..contracts import ClaimReadReceiptJob, ClaimReadStatus, ClaimReceiptReadJob, JobKind
from ..extraction import extract
from ..parsing.receipt import read_receipt
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py

UNREADABLE = "No text could be read from this receipt: fill in its details."


def run(payload: ClaimReadReceiptJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    result = extract(data, ocr=deps.ocr, limits=deps.limits)
    text = (result.text or "").strip()
    claim = payload.claimId
    if result.status != "extracted" or not text:
        log.info("receipt unreadable", extra={"claim_id": claim.root, "status": result.status})
        return [
            _answer(
                ClaimReceiptReadJob(
                    claimId=claim,
                    status=ClaimReadStatus.unreadable,
                    receiptDate=None,
                    amount=None,
                    currency=None,
                    merchant=None,
                    receiptType=None,
                    extractor=result.extractor[:200],
                    errorMessage=(result.error or UNREADABLE)[:500],
                )
            )
        ]
    types = [(t.key, t.label) for t in payload.receiptTypes]
    found = read_receipt(text, types, today=datetime.now(UTC).date())
    log.info(
        "receipt read",
        extra={
            "claim_id": claim.root,
            "extractor": result.extractor,
            "found": [
                name
                for name, value in (
                    ("date", found.receipt_date),
                    ("amount", found.amount),
                    ("currency", found.currency),
                    ("merchant", found.merchant),
                    ("type", found.receipt_type),
                )
                if value is not None
            ],
        },
    )
    return [
        _answer(
            ClaimReceiptReadJob(
                claimId=claim,
                status=ClaimReadStatus.read,
                receiptDate=found.receipt_date,
                amount=found.amount,  # validated into a DecimalAmount
                currency=found.currency,  # validated into a CurrencyCode
                merchant=(found.merchant or None) and found.merchant[:200],
                receiptType=found.receipt_type,
                extractor=result.extractor[:200],
                errorMessage=None,
            )
        )
    ]


def _answer(payload: ClaimReceiptReadJob) -> FollowUp:
    return FollowUp(queue=RESULT_QUEUE, kind=JobKind.claim_receipt_read, payload=payload)
