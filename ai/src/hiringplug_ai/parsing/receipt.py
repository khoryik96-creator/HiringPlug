"""Reading a receipt's text (from a PDF or OCR): its date, total, currency,
merchant and type (Grab fare, food, drinks...).

    read_receipt(text, receipt_types, today) -> ReceiptFields

Rules only, no AI: what is found is a suggestion the claimant checks and can
change. A value not found is None (the claimant fills it in); a doubtful one
is left out rather than guessed.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import date, timedelta
from decimal import Decimal, InvalidOperation

from .dates import MONTHS
from .receipt_words import BRANDS, CURRENCIES, KEYWORDS, SKIP_WORDS


@dataclass(frozen=True)
class ReceiptFields:
    receipt_date: date | None = None
    amount: str | None = None  # "23.50"
    currency: str | None = None  # ISO 4217
    merchant: str | None = None
    receipt_type: str | None = None  # one of the keys offered


# --- dates -------------------------------------------------------------------

_MON = "|".join(sorted(MONTHS, key=len, reverse=True))
_DAY_FIRST = re.compile(r"(?<!\d)(\d{1,2})\s*[/.-]\s*(\d{1,2})\s*[/.-]\s*(\d{4}|\d{2})(?!\d)")
_YEAR_FIRST = re.compile(r"(?<!\d)(\d{4})\s*[/.-]\s*(\d{1,2})\s*[/.-]\s*(\d{1,2})(?!\d)")
_DAY_MONTH_NAME = re.compile(
    rf"(?<![\w])(\d{{1,2}})(?:st|nd|rd|th)?[\s/.-]*({_MON})[a-z]*\.?[\s/.,-]*(\d{{4}}|\d{{2}})(?!\d)",
    re.IGNORECASE,
)
_MONTH_NAME_DAY = re.compile(
    rf"(?<![\w])({_MON})[a-z]*\.?\s+(\d{{1,2}})(?:st|nd|rd|th)?,?\s+(\d{{4}})(?!\d)", re.IGNORECASE
)
_DATE_LABEL = re.compile(r"\b(date|tarikh|dated|booking date|trip date|issued)\b", re.IGNORECASE)


def _year(y: str) -> int:
    n = int(y)
    return n + 2000 if n < 100 else n


def _valid(y: int, m: int, d: int) -> date | None:
    try:
        return date(y, m, d)
    except ValueError:
        return None


def _dates_in(line: str) -> list[date]:
    found: list[date] = []
    for m in _YEAR_FIRST.finditer(line):
        if (d := _valid(int(m[1]), int(m[2]), int(m[3]))) is not None:
            found.append(d)
    for m in _DAY_FIRST.finditer(line):
        day, month, year = int(m[1]), int(m[2]), _year(m[3])
        # Malaysia writes the day first; 11/25/2026 can only be month first.
        d = _valid(year, month, day) or _valid(year, day, month)
        if d is not None:
            found.append(d)
    for m in _DAY_MONTH_NAME.finditer(line):  # the group holds a key of MONTHS
        if (d := _valid(_year(m[3]), MONTHS[m[2].lower()], int(m[1]))) is not None:
            found.append(d)
    for m in _MONTH_NAME_DAY.finditer(line):
        if (d := _valid(int(m[3]), MONTHS[m[1].lower()], int(m[2]))) is not None:
            found.append(d)
    return found


def find_date(lines: list[str], today: date) -> date | None:
    """The receipt's date: on a line saying "Date" if there is one, else the
    first one; never ahead of tomorrow, nor more than three years old."""
    earliest = today - timedelta(days=3 * 366)
    labelled: list[date] = []
    anywhere: list[date] = []
    for line in lines:
        dates = [d for d in _dates_in(line) if earliest <= d <= today + timedelta(days=1)]
        (labelled if _DATE_LABEL.search(line) else anywhere).extend(dates)
    found = labelled or anywhere
    return found[0] if found else None


# --- amounts and currency --------------------------------------------------

_CURRENCY_RE = re.compile(
    "|".join(rf"(?<![A-Za-z])(?P<c{i}>{marker})(?![A-Za-z])" for i, (marker, _) in enumerate(CURRENCIES))
)
_MONEY = re.compile(
    r"(?<![\d.,])(?P<whole>\d{1,3}(?:,\d{3})+|\d{1,7})(?:\.(?P<cents>\d{2}))?(?![\d]|\.\d|,\d{3})"
)

_EXCLUDE = re.compile(
    r"sub\s*-?\s*total|\b(tax|sst|gst|vat|service\s*charge|svc|rounding|round(ing)?\s*adj|change|cash|tendered|"
    r"discount|savings|saved|deposit|points?|qty|quantity|items?|tips?|balance)\b",
    re.IGNORECASE,
)
_STRONG = re.compile(
    r"grand\s*total|total\s*(amount|due|payable|paid|incl|\(incl|rm|myr|sales|charged)|amount\s*(due|paid|payable)|"
    r"nett?\s*total|jumlah(\s*besar)?|balance\s*due|you\s*paid|total\s*fare|fare\s*total",
    re.IGNORECASE,
)
_TOTAL = re.compile(r"\btotal\b", re.IGNORECASE)
_PAID = re.compile(r"\b(paid|payment|amount|fare|bayaran)\b", re.IGNORECASE)
MAX_AMOUNT = Decimal("1000000")


@dataclass(frozen=True)
class _Money:
    value: Decimal
    currency: str | None


def _currency_at(line: str) -> str | None:
    m = _CURRENCY_RE.search(line)
    if m is None:
        return None
    for i, (_, code) in enumerate(CURRENCIES):
        if m.group(f"c{i}") is not None:
            return code
    return None


def _money_in(line: str, need_cents: bool = True) -> list[_Money]:
    currency = _currency_at(line)
    out: list[_Money] = []
    for m in _MONEY.finditer(line):
        if m["cents"] is None and (need_cents or currency is None):
            continue  # a bare whole number is a count, a code or a phone number
        try:
            value = Decimal(m["whole"].replace(",", "") + "." + (m["cents"] or "00"))
        except InvalidOperation:
            continue
        if Decimal(0) < value <= MAX_AMOUNT:
            out.append(_Money(value, currency))
    return out


def find_total(lines: list[str]) -> _Money | None:
    """The receipt's total: on a "grand total", "amount due" or "total paid"
    line, else a "total" line, else what was paid; subtotals, tax, rounding,
    cash given and change never count. Failing those, the largest amount
    written with its currency."""
    best: tuple[int, _Money] | None = None
    for i, line in enumerate(lines):
        if _STRONG.search(line) and not re.search(
            r"sub\s*-?\s*total|total\s*(qty|items?|savings|discount|tax)", line, re.I
        ):
            score = 3
        elif _TOTAL.search(line) and not _EXCLUDE.search(line):
            score = 2
        elif _PAID.search(line) and not _EXCLUDE.search(line):
            score = 1
        else:
            continue
        found = _money_in(line, need_cents=False)
        if not found and i + 1 < len(lines) and not _EXCLUDE.search(lines[i + 1]):
            found = _money_in(lines[i + 1], need_cents=False)  # the amount printed below its label
        for money in found:
            if best is None or (score, money.value) > (best[0], best[1].value):
                best = (score, money)
    if best is not None:
        return best[1]
    marked = [m for line in lines if not _EXCLUDE.search(line) for m in _money_in(line) if m.currency]
    return max(marked, key=lambda m: m.value) if marked else None


def find_currency(lines: list[str], total: _Money | None) -> str | None:
    if total is not None and total.currency:
        return total.currency
    counts: dict[str, int] = {}
    for line in lines:
        if (code := _currency_at(line)) is not None:
            counts[code] = counts.get(code, 0) + 1
    return max(counts, key=lambda c: counts[c]) if counts else None


# --- merchant ----------------------------------------------------------------

# A heading, not the merchant: "TAX INVOICE", "PARKING RECEIPT", "Welcome".
_GENERIC = re.compile(
    r"\b(invoice|receipt|resit|e-?receipt|cash\s*bill)\b"
    r"|^(bill|order|welcome|thank|copy|customer copy|merchant copy|duplicate|reprint)\b"
    r"|^(table|cashier|date|time)\b",
    re.IGNORECASE,
)
_ADDRESS = re.compile(
    r"\b(jalan|jln|lorong|taman|no\.?\s*\d|lot\s*\d|level\s*\d|floor|tel|fax|phone|email|www\.|http|"
    r"gst|sst|reg|company no|co\.? no|\d{5})\b",
    re.IGNORECASE,
)
_REGISTRATION = re.compile(r"\(\s*(\d{6,12}-?[A-Z]?|[A-Z]{2}\d+-[A-Z])\s*\)|\b\d{6,12}-[A-Z]\b")


def _tidy(name: str) -> str:
    name = _REGISTRATION.sub("", name)
    name = re.sub(r"\s+", " ", name).strip(" -:|*#")
    letters = [c for c in name if c.isalpha()]
    if letters and all(c.isupper() for c in letters):  # "NASI KANDAR PELITA SDN BHD" -> title case
        name = " ".join(w.capitalize() for w in name.split())
    return name[:120]


def find_merchant(lines: list[str]) -> str | None:
    text = " ".join(lines).lower()
    for word, brand in BRANDS:
        if re.search(rf"(?<![a-z]){re.escape(word)}(?![a-z])", text):
            return brand
    for line in lines[:8]:
        clean = line.strip()
        letters = sum(c.isalpha() for c in clean)
        if (
            letters < 3
            or letters < len(clean) / 2
            or _GENERIC.search(clean)
            or _ADDRESS.search(clean)
            or _dates_in(clean)
        ):
            continue
        if _money_in(clean):
            continue
        return _tidy(clean) or None
    return None


# --- receipt type ------------------------------------------------------------


def find_type(text: str, types: list[tuple[str, str]]) -> str | None:
    """The receipt type whose words the receipt has the most of, among the
    company's (key, label); None when none show."""
    low = " ".join(text.lower().split())
    scores: dict[str, int] = {}
    for key, label in types:
        words = KEYWORDS.get(key)
        if words is None:  # the company's own type: the words of its name
            words = [(w, 3) for w in re.findall(r"[a-z]{4,}", label.lower()) if w not in SKIP_WORDS]
        score = sum(
            weight for word, weight in words if re.search(rf"(?<![a-z]){re.escape(word)}(?![a-z])", low)
        )
        if score:
            scores[key] = score
    if "grab_fare" in scores and re.search(r"grab\s*food|grabmart", low):
        scores["grab_fare"] -= 2  # GrabFood is a meal, not a ride
    order = [key for key, _ in types]
    ranked = sorted(scores.items(), key=lambda kv: (-kv[1], order.index(kv[0])))
    return ranked[0][0] if ranked and ranked[0][1] > 0 else None


# --- all of it -----------------------------------------------------------------


def read_receipt(text: str, types: list[tuple[str, str]], today: date) -> ReceiptFields:
    lines = [ln for ln in (" ".join(raw.split()) for raw in text.splitlines()) if ln]
    total = find_total(lines)
    return ReceiptFields(
        receipt_date=find_date(lines, today),
        amount=f"{total.value:.2f}" if total else None,
        currency=find_currency(lines, total),
        merchant=find_merchant(lines),
        receipt_type=find_type(text, types),
    )
