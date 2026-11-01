"""The queue client against a real database, connected as the AI worker's own
restricted role: exactly the permissions it has in production."""

from __future__ import annotations

import json
import os
import re
import secrets
from collections.abc import Iterator
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

import psycopg
import pytest
from psycopg import sql

from hiringplug_ai.contracts import DocumentTextExtractedJob, ExtractionStatus, JobKind
from hiringplug_ai.queue import FollowUp, LeaseLostError, Queue

pytestmark = pytest.mark.db

MIGRATIONS = Path(__file__).resolve().parents[2] / "backend" / "db" / "migrations"
ADMIN_URL = os.environ.get("HP_TEST_ADMIN_URL", "postgresql://postgres:postgres@localhost:5432/postgres")
AI_LOGIN = ("hp_ai_pytest", "hp_ai_pytest")


def _with_db(url: str, db: str, user: tuple[str, str] | None = None) -> str:
    parts = urlsplit(url)
    netloc = parts.netloc
    if user:
        netloc = f"{user[0]}:{user[1]}@{netloc.split('@')[-1]}"
    return urlunsplit((parts.scheme, netloc, "/" + db, parts.query, parts.fragment))


def _up_sql(path: Path) -> str:
    """The `-- +goose Up` part of a migration, as one script."""
    text = path.read_text(encoding="utf-8")
    up = text.split("-- +goose Up", 1)[1].split("-- +goose Down", 1)[0]
    return re.sub(r"^-- \+goose Statement(Begin|End)\s*$", "", up, flags=re.M)


@pytest.fixture(scope="module")
def database() -> Iterator[dict[str, str]]:
    if os.environ.get("HP_SKIP_DB_TESTS") == "1":
        pytest.skip("HP_SKIP_DB_TESTS=1")
    name = "hp_pytest_" + secrets.token_hex(4)
    try:
        root = psycopg.connect(ADMIN_URL, autocommit=True)
    except psycopg.OperationalError as exc:
        pytest.fail(
            f"database tests need PostgreSQL ({exc}); start it with `task db:up` or set HP_SKIP_DB_TESTS=1"
        )
    root.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
    admin_url = _with_db(ADMIN_URL, name)
    try:
        with psycopg.connect(admin_url, autocommit=True) as admin:
            for migration in sorted(MIGRATIONS.glob("*.sql")):
                admin.execute(_up_sql(migration).encode())
            admin.execute("SELECT pg_advisory_lock(424243)")
            exists = admin.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", (AI_LOGIN[0],)).fetchone()
            verb = "ALTER" if exists else "CREATE"
            admin.execute(
                sql.SQL(verb + " ROLE {} LOGIN PASSWORD {}").format(
                    sql.Identifier(AI_LOGIN[0]), sql.Literal(AI_LOGIN[1])
                )
            )
            admin.execute(sql.SQL("GRANT hiringplug_ai TO {}").format(sql.Identifier(AI_LOGIN[0])))
            admin.execute("SELECT pg_advisory_unlock(424243)")
            tenant = admin.execute("INSERT INTO tenant (name) VALUES ('pytest') RETURNING id").fetchone()
        assert tenant is not None
        yield {"admin": admin_url, "ai": _with_db(ADMIN_URL, name, AI_LOGIN), "tenant": str(tenant[0])}
    finally:
        root.execute(sql.SQL("DROP DATABASE IF EXISTS {} WITH (FORCE)").format(sql.Identifier(name)))
        root.close()


def enqueue_ai_job(admin_url: str, tenant: str) -> str:
    payload = {
        "documentId": "doc_01jbz6w0m0k8q9x2r3s4t5v6w7",
        "downloadUrl": "http://x",
        "contentType": "application/pdf",
        "fileName": "cv.pdf",
        "sizeBytes": 1,
    }
    with psycopg.connect(admin_url, autocommit=True) as conn:
        row = conn.execute(
            "SELECT job_enqueue(%s, 'ai', 'document.extract_text', %s::jsonb, 'req_pytest',"
            " now() - interval '1 hour')",
            (tenant, json.dumps(payload)),
        ).fetchone()
    assert row is not None
    return str(row[0])


def result_payload(status: str = "extracted") -> DocumentTextExtractedJob:
    return DocumentTextExtractedJob(
        documentId="doc_01jbz6w0m0k8q9x2r3s4t5v6w7",
        status=ExtractionStatus(status),
        text="CV text",
        pageCount=1,
        extractor="test@1",
        errorMessage=None,
        warnings=[],
    )


def job_row(admin_url: str, job_id: str) -> tuple[str, int, str | None]:
    with psycopg.connect(admin_url) as conn:
        row = conn.execute("SELECT status, attempts, last_error FROM job WHERE id = %s", (job_id,)).fetchone()
    assert row is not None
    return row[0], row[1], row[2]


def test_claim_then_complete_queues_the_result_for_go(database: dict[str, str]) -> None:
    job_id = enqueue_ai_job(database["admin"], database["tenant"])
    queue = Queue(database["ai"], "pytest-worker")
    job = queue.claim(lease_seconds=60)
    assert job is not None
    assert job.id == job_id
    assert job.request_id == "req_pytest"

    queue.complete(job, [FollowUp("core", JobKind.document_text_extracted, result_payload())])
    assert job_row(database["admin"], job_id)[0] == "done"
    with psycopg.connect(database["admin"]) as conn:
        follow_up = conn.execute(
            "SELECT queue, request_id, payload->>'status' FROM job WHERE kind = 'document.text_extracted'"
        ).fetchone()
    assert follow_up == ("core", "req_pytest", "extracted"), "the result carries the original request ID"
    queue.close()


def test_a_failure_is_recorded_for_a_later_retry(database: dict[str, str]) -> None:
    job_id = enqueue_ai_job(database["admin"], database["tenant"])
    queue = Queue(database["ai"], "pytest-worker")
    job = queue.claim(lease_seconds=60)
    assert job is not None
    assert queue.fail(job, "RuntimeError: provider timeout") == "pending"
    status, attempts, error = job_row(database["admin"], job_id)
    assert (status, attempts, error) == ("pending", 1, "RuntimeError: provider timeout")
    queue.close()


def test_after_losing_the_lease_nothing_is_committed(database: dict[str, str]) -> None:
    job_id = enqueue_ai_job(database["admin"], database["tenant"])
    queue = Queue(database["ai"], "pytest-worker")
    job = queue.claim(lease_seconds=60)
    assert job is not None
    with psycopg.connect(database["admin"], autocommit=True) as admin:
        admin.execute("UPDATE job SET locked_by = 'someone-else' WHERE id = %s", (job_id,))
        before = admin.execute("SELECT count(*) FROM job").fetchone()
    with pytest.raises(LeaseLostError):
        queue.complete(job, [FollowUp("core", JobKind.document_text_extracted, result_payload())])
    with psycopg.connect(database["admin"]) as admin:
        assert admin.execute("SELECT count(*) FROM job").fetchone() == before, "the follow-up was rolled back"
    queue.close()


def test_the_ai_role_cannot_read_candidate_data(database: dict[str, str]) -> None:
    with psycopg.connect(database["ai"]) as conn, pytest.raises(psycopg.errors.InsufficientPrivilege):
        conn.execute("SELECT count(*) FROM candidate")
