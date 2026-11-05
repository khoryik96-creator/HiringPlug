"""Where a candidate lives: an ISO country code, from (in order) their
address or location line, place names near the top of the CV, or their
phone number's country code. Like Kano's check, but narrower: an employer's
"Sdn Bhd" or an address further down the CV says nothing about where the
candidate lives. Nothing found means not known (shown as NA)."""

from __future__ import annotations

import re

from .layout import Line

COUNTRIES = {
    "malaysia": "MY", "singapore": "SG", "indonesia": "ID", "philippines": "PH", "thailand": "TH",
    "vietnam": "VN", "viet nam": "VN", "india": "IN", "brunei": "BN", "hong kong": "HK",
    "australia": "AU", "china": "CN", "japan": "JP", "south korea": "KR", "taiwan": "TW",
    "united kingdom": "GB", "united states": "US", "usa": "US", "united arab emirates": "AE",
    "uae": "AE", "qatar": "QA", "saudi arabia": "SA", "bangladesh": "BD", "pakistan": "PK",
    "sri lanka": "LK", "nepal": "NP", "myanmar": "MM", "cambodia": "KH", "new zealand": "NZ",
    "canada": "CA",
}  # fmt: skip

# Malaysian states, cities and areas people write as their address.
MY_PLACES = (
    "kuala lumpur", "selangor", "penang", "pulau pinang", "johor bahru", "johor", "perak", "ipoh",
    "kedah", "kelantan", "terengganu", "pahang", "melaka", "malacca", "negeri sembilan", "seremban",
    "sabah", "kota kinabalu", "sarawak", "kuching", "putrajaya", "cyberjaya", "petaling jaya",
    "shah alam", "subang jaya", "klang", "puchong", "cheras", "ampang", "bangsar", "mont kiara",
    "damansara", "kajang", "seri kembangan", "bukit jalil", "setapak", "wangsa maju",
)  # fmt: skip

_COUNTRY_RE = re.compile(
    r"\b(" + "|".join(sorted(map(re.escape, COUNTRIES), key=len, reverse=True)) + r")\b", re.I
)
_MY_RE = re.compile(
    r"\b(" + "|".join(sorted(map(re.escape, MY_PLACES), key=len, reverse=True)) + r")\b", re.I
)
_KL_RE = re.compile(r"\bKL\b")  # in capitals only
# Country calling codes: the longest first (+673 before +67...).
_CALLING = {
    "+673": "BN", "+60": "MY", "+65": "SG", "+62": "ID", "+63": "PH", "+66": "TH", "+84": "VN",
    "+91": "IN", "+852": "HK", "+61": "AU", "+86": "CN", "+81": "JP", "+82": "KR", "+44": "GB",
    "+971": "AE", "+880": "BD",
}  # fmt: skip
_MY_MOBILE = re.compile(r"^\(?0?1\d\)?[\s-]?\d{3,4}[\s-]?\d{4}$")


def _place(text: str) -> tuple[str, str] | None:
    if m := _COUNTRY_RE.search(text):
        return COUNTRIES[m.group(1).lower()], m.group(1)
    if m := _MY_RE.search(text) or _KL_RE.search(text):
        return "MY", m.group(0)
    return None


def residence_country(
    lines: list[Line], location: tuple[str, str] | None, phone: tuple[str, str] | None
) -> tuple[str, str] | None:
    """(code, quote) or None."""
    if location and (found := _place(location[1])):
        return found
    for line in lines[:12]:  # the CV's header, where people put their address
        if found := _place(line.text):
            return found
    if phone:
        number = phone[1].strip()
        compact = number.replace(" ", "").replace("-", "")
        for prefix in sorted(_CALLING, key=len, reverse=True):
            if compact.startswith(prefix):
                return _CALLING[prefix], number
        if _MY_MOBILE.match(number):  # 012-345 6789: a Malaysian mobile
            return "MY", number
    return None
