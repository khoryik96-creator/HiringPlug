"""Sending a paid request again only when the provider did no work."""

from __future__ import annotations

import time
from collections.abc import Callable

# 429: rate limited; 529: overloaded. Both say the model did not run, so a
# second try cannot bill twice. A 500 or a timeout may have run it: never sent
# again (CV Studio's rule).
REJECTED_BEFORE_WORK = frozenset({429, 529})
ATTEMPTS = 3


class RejectedError(Exception):
    """The provider turned the request away before doing any work."""

    def __init__(self, retry_after: str | None) -> None:
        super().__init__("rejected before work")
        self.retry_after = retry_after


def delay(retry_after: str | None, attempt: int) -> float:
    """Retry-After when given (1-30 s), else 1 s, 2 s..."""
    try:
        return float(max(1, min(30, int(str(retry_after).strip()))))
    except (TypeError, ValueError):
        return min(8.0, 2.0**attempt)


def with_retries[T](
    send: Callable[[], T], on_give_up: Callable[[], Exception], sleep: Callable[[float], None] = time.sleep
) -> T:
    for attempt in range(ATTEMPTS):
        try:
            return send()
        except RejectedError as exc:
            if attempt == ATTEMPTS - 1:
                raise on_give_up() from exc
            sleep(delay(exc.retry_after, attempt))
    raise AssertionError("unreachable")
