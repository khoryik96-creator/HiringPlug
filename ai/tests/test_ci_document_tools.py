from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[2] / ".github/scripts/install-document-tools.sh"


def run_installer(
    tmp_path: Path, failures: str, *, fail_at: str = "download"
) -> tuple[subprocess.CompletedProcess[str], list[str]]:
    # Replace only system commands: no packages, network or /etc files are changed.
    shim = tmp_path / "shim"
    shim.write_text(
        """#!/usr/bin/env bash
set -eu
name=$(basename "$0")
echo "$name $*" >> "$TRACE"
case "$name" in
  timeout) shift 2; exec "$@" ;;
  sed) exit 0 ;;
  apt-get)
    if { [[ "$FAIL_AT" == download && " $* " == *' --download-only '* ]]; } ||
       { [[ "$FAIL_AT" == update && " $* " == *' update '* ]]; } ||
       { [[ "$FAIL_AT" == configure && " $* " == *' --no-download '* ]]; }; then
      count=$(cat "$COUNT" 2>/dev/null || echo 0)
      echo $((count + 1)) > "$COUNT"
      if (( count < FAILURES )); then exit 124; fi
    fi ;;
esac
"""
    )
    shim.chmod(0o755)
    for name in ("apt-get", "timeout", "sed"):
        (tmp_path / name).symlink_to(shim)
    trace = tmp_path / "trace"
    env = {
        **os.environ,
        "PATH": f"{tmp_path}:{os.environ['PATH']}",
        "TRACE": str(trace),
        "COUNT": str(tmp_path / "count"),
        "FAILURES": failures,
        "FAIL_AT": fail_at,
    }
    bash = shutil.which("bash")
    assert bash is not None
    result = subprocess.run(  # noqa: S603
        [bash, str(SCRIPT)], env=env, capture_output=True, text=True, timeout=5, check=False
    )
    return result, trace.read_text().splitlines()


def test_success_installs_only_after_bounded_download(tmp_path: Path) -> None:
    result, trace = run_installer(tmp_path, "0")
    assert result.returncode == 0, result.stderr
    assert sum(line.startswith("apt-get") and "--download-only" in line for line in trace) == 1
    assert any(line.startswith("timeout") and "30s" in line for line in trace)
    assert any(line.startswith("timeout") and "90s" in line for line in trace)
    assert "--no-download" in trace[-1]
    assert "libreoffice-writer-nogui" in trace[-1]


@pytest.mark.parametrize("fail_at", ["download", "update"])
def test_network_failure_retries_before_installing(tmp_path: Path, fail_at: str) -> None:
    result, trace = run_installer(tmp_path, "1", fail_at=fail_at)
    assert result.returncode == 0, result.stderr
    assert "::warning::" in result.stdout
    downloads = sum(line.startswith("apt-get") and "--download-only" in line for line in trace)
    assert downloads == (2 if fail_at == "download" else 1)
    assert "--no-download" in trace[-1]


@pytest.mark.parametrize("fail_at", ["download", "update"])
def test_persistent_network_failure_never_configures_packages(tmp_path: Path, fail_at: str) -> None:
    result, trace = run_installer(tmp_path, "2", fail_at=fail_at)
    assert result.returncode != 0
    assert not any("--no-download" in line for line in trace)


def test_package_configuration_failure_is_not_hidden_or_retried(tmp_path: Path) -> None:
    result, trace = run_installer(tmp_path, "1", fail_at="configure")
    assert result.returncode != 0
    assert sum(line.startswith("apt-get") and "--no-download" in line for line in trace) == 1
