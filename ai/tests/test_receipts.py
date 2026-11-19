"""Reading receipts for expense claims: date, total, currency, merchant, type."""

from __future__ import annotations

import io
import os
from datetime import UTC, date, datetime, timedelta

import pypdfium2
import pytest

from hiringplug_ai.contracts import ClaimReadReceiptJob, ClaimReceiptReadJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS, TesseractOcr
from hiringplug_ai.jobs import Deps
from hiringplug_ai.jobs import read_receipt as job
from hiringplug_ai.parsing.receipt import read_receipt

from .documents import text_pdf

TODAY = date(2026, 11, 19)
TYPES = [
    ("grab_fare", "Grab / e-hailing fare"),
    ("food", "Food"),
    ("drinks", "Drinks"),
    ("parking_toll", "Parking & toll"),
    ("fuel", "Fuel"),
    ("travel", "Flights, trains & hotels"),
    ("phone", "Phone & internet"),
    ("office", "Office supplies"),
    ("other", "Other"),
]

GRAB_RIDE = """Grab
Your Grab E-Receipt
Hope you enjoyed your ride!
Total Paid RM 23.00
Booking ID: A-5ZK3GH8WWJ9A
Booking Date 03 Nov 2026 09:41 AM
Ride Type GrabCar
Pick-up location: KL Sentral
Drop-off location: Menara Acme, KLCC
Fare RM 20.00
Platform & partner fee RM 2.00
Tolls RM 1.00
TOTAL RM 23.00
Paid by Visa ****1234"""

RESTAURANT = """RESTORAN NASI KANDAR PELITA SDN BHD
(123456-X)
NO 149, JALAN AMPANG, 50450 KUALA LUMPUR
TEL: 03-2162 5532
SST ID: W10-1808-31000123
TAX INVOICE
Inv No: 120345 Date: 05/11/2026 13:22
Table: 12 Pax: 3
1 Nasi Kandar Ayam 15.90
2 Teh Tarik 6.40
1 Roti Canai 2.20
SUBTOTAL 24.50
SST 6% 1.47
Rounding -0.02
TOTAL 25.95
CASH 50.00
CHANGE 24.05
THANK YOU, PLEASE COME AGAIN"""

COFFEE = """ZUS COFFEE
ZUS Coffee - Mid Valley
Order #A123
17-Nov-2026 08:15
1x Spanish Latte (L) RM 12.90
1x Americano (M) RM 9.90
Subtotal RM 22.80
Total RM 22.80
Paid via TNG eWallet"""

PARKING = """PARKING RECEIPT
MID VALLEY MEGAMALL
Entry: 18/11/2026 09:02
Exit : 18/11/2026 12:47
Duration: 3h 45m
Amount Paid: RM 8.00"""


def test_a_grab_ride_receipt() -> None:
    r = read_receipt(GRAB_RIDE, TYPES, TODAY)
    assert (r.receipt_date, r.amount, r.currency, r.merchant, r.receipt_type) == (
        date(2026, 11, 3),
        "23.00",
        "MYR",
        "Grab",
        "grab_fare",
    )


def test_a_restaurant_receipt_counts_the_total_not_the_cash_given_or_the_subtotal() -> None:
    r = read_receipt(RESTAURANT, TYPES, TODAY)
    assert r.receipt_date == date(2026, 11, 5), "Malaysia writes the day first"
    assert r.amount == "25.95"
    assert r.merchant == "Restoran Nasi Kandar Pelita Sdn Bhd"
    assert r.receipt_type == "food"
    assert r.currency is None, "no currency written: the claim's default stands"


def test_a_coffee_receipt_is_drinks_even_paid_with_touch_n_go() -> None:
    r = read_receipt(COFFEE, TYPES, TODAY)
    assert (r.receipt_date, r.amount, r.currency, r.merchant, r.receipt_type) == (
        date(2026, 11, 17),
        "22.80",
        "MYR",
        "ZUS Coffee",
        "drinks",
    )


def test_a_parking_ticket() -> None:
    r = read_receipt(PARKING, TYPES, TODAY)
    assert (r.receipt_date, r.amount, r.merchant, r.receipt_type) == (
        date(2026, 11, 18),
        "8.00",
        "Mid Valley Megamall",
        "parking_toll",
    )


def test_grabfood_is_a_meal_not_a_ride() -> None:
    r = read_receipt(
        "GrabFood\nOrder from Ayam Penyet Ria\nDelivered 12 Nov 2026\nTotal RM 35.60", TYPES, TODAY
    )
    assert (r.merchant, r.receipt_type, r.amount) == ("GrabFood", "food", "35.60")


def test_currencies_and_date_forms() -> None:
    sg = read_receipt("Ya Kun Kaya Toast\nNov 15, 2026\nGRAND TOTAL S$ 18.50\nThank you", TYPES, TODAY)
    assert (sg.receipt_date, sg.amount, sg.currency) == (date(2026, 11, 15), "18.50", "SGD")
    iso = read_receipt("Hotel Stay\nDate: 2026-11-10\nAmount due USD 1,234.50", TYPES, TODAY)
    assert (iso.receipt_date, iso.amount, iso.currency, iso.receipt_type) == (
        date(2026, 11, 10),
        "1234.50",
        "USD",
        "travel",
    )
    month_first = read_receipt("Store\n11/14/2026\nTotal 9.90", TYPES, TODAY)
    assert month_first.receipt_date == date(2026, 11, 14), "only a month-first reading makes sense"
    whole = read_receipt("Petronas Setia Alam\nRON95 25.00 litres\nTOTAL RM50", TYPES, TODAY)
    assert (whole.amount, whole.merchant, whole.receipt_type) == ("50.00", "Petronas", "fuel")


def test_doubtful_values_are_left_out() -> None:
    future = read_receipt("Shop\nDate: 25/12/2026\nTotal 10.00", TYPES, TODAY)
    assert future.receipt_date is None, "a day ahead of today is a misreading"
    old = read_receipt("Shop\nDate: 01/01/2019\nTotal 10.00", TYPES, TODAY)
    assert old.receipt_date is None
    nothing = read_receipt("~ ,. ;;\n|||", TYPES, TODAY)
    assert nothing == type(nothing)(), "nothing found, nothing guessed"
    counts = read_receipt("Total Qty: 4\nItems 4\nCASH 100.00", TYPES, TODAY)
    assert counts.amount is None, "a count or the cash given is not the total"


def test_a_company_type_matches_on_the_words_of_its_name() -> None:
    types = [*TYPES, ("team_building", "Team building activity")]
    r = read_receipt("Escape Room KL\nTeam building package for 8\nTotal RM 400.00", types, TODAY)
    assert r.receipt_type == "team_building"
    assert read_receipt(GRAB_RIDE, [("food", "Food")], TODAY).receipt_type is None, "only the company's types"


def receipt_job(data: bytes, content_type: str = "application/pdf") -> ClaimReceiptReadJob:
    payload = ClaimReadReceiptJob.model_validate(
        {
            "claimId": "clm_01jbz6w0m0k8q9x2r3s4t5v6w7",
            "downloadUrl": "http://api.test/internal/files/x?sig=1",
            "contentType": content_type,
            "fileName": "receipt",
            "sizeBytes": len(data),
            "receiptTypes": [{"key": k, "label": label} for k, label in TYPES],
        }
    )
    deps = Deps(
        download=lambda url, limit: data,
        ocr=TesseractOcr(),
        limits=DEFAULT_LIMITS,
        max_download_bytes=10_000_000,
    )
    [follow_up] = job.run(payload, deps)
    assert (follow_up.queue, follow_up.kind) == ("core", JobKind.claim_receipt_read)
    result = follow_up.payload
    assert isinstance(result, ClaimReceiptReadJob)
    # The answer must satisfy the contract the Go side validates against.
    return ClaimReceiptReadJob.model_validate(result.model_dump(mode="json"))


def test_a_pdf_receipt_is_read_and_sent_back() -> None:
    result = receipt_job(text_pdf(GRAB_RIDE.splitlines()))
    assert result.status.value == "read"
    assert result.amount is not None
    assert result.amount.root == "23.00"
    assert result.receiptType == "grab_fare"
    assert result.merchant == "Grab"
    assert result.errorMessage is None


def test_a_file_without_text_is_unreadable() -> None:
    result = receipt_job(b"\x89PNG\r\n\x1a\nnot really a picture", "image/png")
    assert result.status.value == "unreadable"
    assert result.amount is None
    assert result.errorMessage


@pytest.mark.ocr
def test_a_photo_of_a_receipt_is_read_with_ocr() -> None:
    day = datetime.now(UTC).date() - timedelta(days=3)  # the job checks the date against today
    lines = RESTAURANT.replace("05/11/2026", day.strftime("%d/%m/%Y")).splitlines()
    page = pypdfium2.PdfDocument(text_pdf(lines, font_size=14))[0]
    photo = io.BytesIO()
    page.render(scale=200 / 72).to_pil().convert("L").save(photo, format="JPEG", quality=85)
    result = receipt_job(photo.getvalue(), "image/jpeg")
    assert result.status.value == "read"
    assert result.extractor.startswith("tesseract")
    assert result.amount is not None
    assert result.amount.root == "25.95"
    assert result.receiptDate == day
    assert result.receiptType == "food"


def test_tesseract_runs_one_thread_so_jobs_do_not_fight_over_cores(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("OMP_THREAD_LIMIT", raising=False)
    TesseractOcr()
    assert os.environ["OMP_THREAD_LIMIT"] == "1"
    monkeypatch.setenv("OMP_THREAD_LIMIT", "4")
    TesseractOcr()
    assert os.environ["OMP_THREAD_LIMIT"] == "4", "a server's own setting stands"
