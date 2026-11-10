from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[2] / ".github/scripts/install-document-tools.sh"


def run_installer(
    tmp_path: Path,
    failures: str,
    *,
    fail_at: str = "download",
    source_file: str = "apt-mirrors.txt",
) -> tuple[subprocess.CompletedProcess[str], list[str]]:
    # Redirect /etc/apt into a fixture and replace network/package commands.
    # Real sed must update the fixture, including runners using mirror+file.
    apt_dir = tmp_path / "apt"
    source = apt_dir / source_file
    source.parent.mkdir(parents=True)
    if source_file.endswith(".sources"):
        source.write_text(
            "Types: deb\nURIs: http://azure.archive.ubuntu.com/ubuntu\nSuites: resolute\nComponents: main\n"
        )
    else:
        source.write_text("http://azure.archive.ubuntu.com/ubuntu\nhttp://security.ubuntu.com/ubuntu\n")
        reference = apt_dir / "sources.list.d/ubuntu.sources"
        reference.parent.mkdir()
        reference.write_text(f"Types: deb\nURIs: mirror+file:{source}\nSuites: resolute\n")
    script = tmp_path / "installer.sh"
    script.write_text(SCRIPT.read_text().replace("/etc/apt", str(apt_dir)))
    shim = tmp_path / "shim"
    shim.write_text(
        """#!/usr/bin/env bash
set -eu
name=$(basename "$0")
echo "$name $*" >> "$TRACE"
case "$name" in
  timeout) shift 2; exec "$@" ;;
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
    for name in ("apt-get", "timeout"):
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
        [bash, str(script)], env=env, capture_output=True, text=True, timeout=5, check=False
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


@pytest.mark.parametrize("source_file", ["apt-mirrors.txt", "sources.list.d/ubuntu.sources"])
def test_retry_switches_the_actual_mirror_file(tmp_path: Path, source_file: str) -> None:
    result, _ = run_installer(tmp_path, "1", source_file=source_file)
    assert result.returncode == 0, result.stderr
    content = (tmp_path / "apt" / source_file).read_text()
    assert "http://archive.ubuntu.com/ubuntu\n" in content
    assert "azure.archive.ubuntu.com" not in content
    if source_file == "apt-mirrors.txt":
        assert "http://security.ubuntu.com/ubuntu\n" in content
        assert "mirror+file:" in (tmp_path / "apt/sources.list.d/ubuntu.sources").read_text()
