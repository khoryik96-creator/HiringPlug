"""The AI worker says that it runs, and which version, for Settings > System."""

from __future__ import annotations

import threading
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import psycopg

from hiringplug_ai.heartbeat import NAME, Heartbeat, version


class FakeConn:
    def __init__(self, fail: bool = False) -> None:
        self.calls: list[tuple[str, tuple[Any, ...]]] = []
        self.fail = fail
        self.closed = False
        self.autocommit = False

    def execute(self, sql: str, params: tuple[Any, ...]) -> None:
        if self.fail:
            raise psycopg.OperationalError("the database went away")
        self.calls.append((sql, params))

    def close(self) -> None:
        self.closed = True


def test_it_reports_its_name_version_and_start() -> None:
    conn = FakeConn()
    started = datetime(2026, 11, 26, 9, 0, tzinfo=UTC)
    beat = Heartbeat("postgres://x", "1.52.0", connect=lambda _: conn, now=lambda: started)
    assert beat.beat()
    assert beat.beat()
    assert conn.autocommit
    assert conn.calls == [("SELECT server_process_seen(%s, %s, %s)", (NAME, "1.52.0", started))] * 2


def test_a_failed_report_is_tried_again_on_a_new_connection() -> None:
    conns = [FakeConn(fail=True), FakeConn()]
    made: list[FakeConn] = []

    def connect(_: str) -> FakeConn:
        made.append(conns[len(made)])
        return made[-1]

    beat = Heartbeat("postgres://x", "1.52.0", connect=connect)
    assert not beat.beat()
    assert conns[0].closed
    assert beat.beat()
    assert len(conns[1].calls) == 1


def test_it_reports_at_once_and_stops_when_told() -> None:
    conn = FakeConn()
    stop = threading.Event()
    beat = Heartbeat("postgres://x", "1.52.0", connect=lambda _: conn)
    stop.set()
    beat.run(stop, every=60)
    assert len(conn.calls) == 1


def test_the_version_is_the_nearest_version_file(tmp_path: Path) -> None:
    (tmp_path / "VERSION").write_text("1.52.0\n", encoding="utf-8")
    inner = tmp_path / "app" / "src" / "hiringplug_ai"
    inner.mkdir(parents=True)
    assert version(inner / "heartbeat.py") == "1.52.0"
    (tmp_path / "VERSION").write_text("not a version", encoding="utf-8")
    assert version(inner / "heartbeat.py") == "unknown"


def test_the_checkout_version_is_found() -> None:
    root = Path(__file__).resolve().parents[2]
    assert version() == (root / "VERSION").read_text(encoding="utf-8").strip()
