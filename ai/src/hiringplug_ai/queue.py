"""The Python side of the job queue shared with Go.

The queue's rules live in PostgreSQL functions (migration 00003), called
identically from Go and Python: job_claim, job_enqueue, job_complete, job_fail.
This worker's database role may call those functions and nothing else.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any

import psycopg
from pydantic import BaseModel

from .contracts import JobKind

AI_QUEUE = "ai"


@dataclass(frozen=True)
class ClaimedJob:
    id: str
    tenant_id: str
    kind: str
    payload: dict[str, Any]
    attempts: int
    max_attempts: int
    request_id: str | None
    # 1 (someone waits on screen) to 9 (background upkeep); its results inherit it.
    priority: int = 5


@dataclass(frozen=True)
class FollowUp:
    """A job to queue when the current one completes (e.g. a result for Go)."""

    queue: str
    kind: JobKind
    payload: BaseModel


class LeaseLostError(RuntimeError):
    """Another worker took this job after our lease expired; our results were discarded."""


class Queue:
    def __init__(self, conninfo: str, worker_id: str) -> None:
        self._conninfo = conninfo
        self.worker_id = worker_id
        self._conn: psycopg.Connection[Any] | None = None
        self._listener: psycopg.Connection[Any] | None = None

    def _connection(self) -> psycopg.Connection[Any]:
        if self._conn is None or self._conn.closed:
            self._conn = psycopg.connect(self._conninfo, autocommit=True)
        return self._conn

    def close(self) -> None:
        for conn in (self._conn, self._listener):
            if conn is not None:
                conn.close()

    def claim(self, lease_seconds: int, queue: str = AI_QUEUE) -> ClaimedJob | None:
        row = (
            self._connection()
            .execute(
                "SELECT id, tenant_id, kind, payload, attempts, max_attempts, request_id, priority"
                " FROM job_claim(%s, %s, %s)",
                (queue, self.worker_id, lease_seconds),
            )
            .fetchone()
        )
        if row is None:
            return None
        return ClaimedJob(*row)

    def complete(self, job: ClaimedJob, follow_ups: list[FollowUp]) -> None:
        """Queue the follow-up jobs and mark this job done, atomically."""
        conn = self._connection()
        with conn.transaction():
            for f in follow_ups:
                conn.execute(
                    "SELECT job_enqueue(%s, %s, %s, %s::jsonb, %s, p_priority => %s::smallint)",
                    (
                        job.tenant_id,
                        f.queue,
                        f.kind.value,
                        json.dumps(f.payload.model_dump(mode="json")),
                        job.request_id,
                        job.priority,  # a result is as urgent as the job that made it
                    ),
                )
            row = conn.execute("SELECT job_complete(%s, %s)", (job.id, self.worker_id)).fetchone()
            if row is None or not row[0]:
                raise LeaseLostError(job.id)  # rolls back the follow-ups

    def fail(self, job: ClaimedJob, error: str) -> str:
        """Record a failure. Returns the job's new status: 'pending' (retried later) or 'dead'."""
        row = (
            self._connection()
            .execute("SELECT job_fail(%s, %s, %s)", (job.id, self.worker_id, error))
            .fetchone()
        )
        return str(row[0]) if row and row[0] else "unknown"

    def wait_for_work(self, timeout: float) -> None:
        """Sleep until a job is queued on the AI queue (Postgres NOTIFY) or timeout passes."""
        if self._listener is None or self._listener.closed:
            self._listener = psycopg.connect(self._conninfo, autocommit=True)
            self._listener.execute(f"LISTEN job_{AI_QUEUE}")  # a constant, never user input
        for _ in self._listener.notifies(timeout=timeout, stop_after=1):
            break
