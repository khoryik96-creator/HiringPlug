"""Says every half minute that the AI worker runs, and which version of
HyPlug it runs (the VERSION file), through server_process_seen(): Settings
> System shows an AI worker that stopped, or one still on the old version
after an update. A report that fails is logged and tried at the next beat."""

from __future__ import annotations

import contextlib
import logging
import re
import threading
from collections.abc import Callable
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import psycopg

log = logging.getLogger(__name__)

NAME = "ai-worker"
EVERY = 30.0  # seconds

_PLAIN = re.compile(r"^\d+\.\d+\.\d+$")


def version(start: Path | None = None) -> str:
    """The VERSION file nearest above start (this package): the checkout's
    in development, /app/VERSION in the image; "unknown" when there is none."""
    here = (start or Path(__file__)).resolve()
    for folder in [here, *here.parents]:
        candidate = folder / "VERSION"
        try:
            text = candidate.read_text(encoding="utf-8").strip()
        except OSError:
            continue
        if _PLAIN.match(text):
            return text
    return "unknown"


class Heartbeat:
    """Reports that this process runs; keeps one connection between beats."""

    def __init__(
        self,
        database_url: str,
        running_version: str,
        *,
        connect: Callable[[str], Any] = psycopg.connect,
        now: Callable[[], datetime] = lambda: datetime.now(UTC),
    ) -> None:
        self._url = database_url
        self._version = running_version
        self._connect = connect
        self._started = now()
        self._conn: Any = None

    def beat(self) -> bool:
        """Reports once; False (logged) when the database could not be told."""
        try:
            if self._conn is None:
                self._conn = self._connect(self._url)
                self._conn.autocommit = True
            self._conn.execute("SELECT server_process_seen(%s, %s, %s)", (NAME, self._version, self._started))
        except psycopg.Error:
            conn, self._conn = self._conn, None
            if conn is not None:
                with contextlib.suppress(psycopg.Error):
                    conn.close()
            log.warning("could not report that the AI worker runs", exc_info=True)
            return False
        return True

    def run(self, stop: threading.Event, every: float = EVERY) -> None:
        """Reports at once and then every `every` seconds until stop is set."""
        self.beat()
        while not stop.wait(every):
            self.beat()
