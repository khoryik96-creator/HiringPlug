"""The download of a job's file: what is worth trying again, and what is not
(BUG-003: a file gone or a dead link was retried five times, then the
document waited forever). Its messages never hold the signed link."""

from __future__ import annotations

import threading
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from hiringplug_ai.download import DownloadError, DownloadGoneError, FileTooLargeError, GiveUpError, download

SECRET = "sig=0123456789abcdef"


class _Answer(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        status = int(self.path.split("?")[0].split("/")[2])
        self.send_response(status)
        body = b"%PDF-1.4 synthetic" if status == 200 else b"no"
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_: object) -> None:
        pass


@pytest.fixture
def server() -> Iterator[str]:
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _Answer)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    yield f"http://127.0.0.1:{httpd.server_address[1]}"
    httpd.shutdown()


def test_the_file_is_downloaded(server: str) -> None:
    assert download(f"{server}/files/200?{SECRET}", 1000) == b"%PDF-1.4 synthetic"


@pytest.mark.parametrize("status", [403, 404, 410])
def test_a_file_gone_or_a_dead_link_is_given_up_at_once(server: str, status: int) -> None:
    with pytest.raises(DownloadGoneError) as caught:
        download(f"{server}/files/{status}?{SECRET}", 1000)
    assert isinstance(caught.value, GiveUpError)
    assert f"HTTP {status}" in str(caught.value)
    assert SECRET not in str(caught.value)
    assert server not in str(caught.value)


@pytest.mark.parametrize("status", [500, 502, 503])
def test_a_server_error_is_tried_again(server: str, status: int) -> None:
    with pytest.raises(DownloadError) as caught:
        download(f"{server}/files/{status}?{SECRET}", 1000)
    assert not isinstance(caught.value, GiveUpError)
    assert str(caught.value) == f"download failed with HTTP {status}"


def test_an_unreachable_server_is_tried_again_and_its_link_never_shown() -> None:
    with pytest.raises(DownloadError) as caught:
        download(f"http://127.0.0.1:9/internal/files/tnt_a/doc_b?{SECRET}", 1000, timeout_seconds=2)
    assert not isinstance(caught.value, GiveUpError)
    assert SECRET not in str(caught.value)
    assert "doc_b" not in str(caught.value)
    assert caught.value.__cause__ is None, "no chained message with the link"
    assert caught.value.__suppress_context__


def test_a_file_too_large_is_given_up(server: str) -> None:
    with pytest.raises(FileTooLargeError):
        download(f"{server}/files/200?{SECRET}", 5)
