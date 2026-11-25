"""The server's paid-AI switch (Settings > System; migration 00201).

Paused, no paid AI call is made on the server: not through a company's own
key, not through the server's provider, not for a job queued before it was
paused. The worker asks the database (ai_paid_paused(), the only thing its
role may read besides the queue) at most every few seconds, not per call.
HP_AI_PAID=off pauses it whatever the database says.
"""

from __future__ import annotations

import contextlib
import logging
import threading
import time
from collections.abc import Callable
from typing import Any

import psycopg

from .providers import ProviderError

log = logging.getLogger(__name__)

PAID_PAUSED = "paid AI is paused on this server (Settings > System), so the AI was not asked"


class PaidAIPausedError(ProviderError):
    """The server's paid AI is paused: a job queued before still makes no
    paid call. Its handler treats it as the AI not answering: a CV is read by
    the rules alone, an assistant question answered with why."""


class PaidSwitch:
    """Whether paid AI is paused, cached for `ttl` seconds; shared by the
    worker's slots (threads)."""

    def __init__(
        self,
        database_url: str,
        *,
        env_off: bool = False,
        ttl: float = 5.0,
        clock: Callable[[], float] = time.monotonic,
        connect: Callable[[str], Any] = psycopg.connect,
    ) -> None:
        self._url = database_url
        self._env_off = env_off
        self._ttl = ttl
        self._clock = clock
        self._connect = connect
        self._lock = threading.Lock()
        self._conn: Any = None
        self._paused: bool | None = None
        self._read_at = 0.0

    def paused(self) -> bool:
        if self._env_off:
            return True
        with self._lock:
            now = self._clock()
            if self._paused is not None and now - self._read_at < self._ttl:
                return self._paused
            try:
                if self._conn is None:
                    self._conn = self._connect(self._url)
                    self._conn.autocommit = True
                row = self._conn.execute("SELECT ai_paid_paused()").fetchone()
                self._paused, self._read_at = bool(row[0]), now
            except psycopg.Error:
                self._close()
                # Unknown: the last answer stands; never known, no paid call.
                log.warning("could not read the paid-AI switch", exc_info=True)
                if self._paused is None:
                    return True
            return self._paused

    def _close(self) -> None:
        conn, self._conn = self._conn, None
        if conn is not None:
            with contextlib.suppress(psycopg.Error):
                conn.close()
