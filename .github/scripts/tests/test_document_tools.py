from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "install-document-tools.sh"


class DocumentToolsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)

    def fixture(
        self,
        failures: int = 0,
        fail_at: str = "download",
        source_file: str = "apt-mirrors.txt",
        mirror: str = "http://azure.archive.ubuntu.com/ubuntu",
    ) -> tuple[Path, dict[str, str]]:
        root = Path(tempfile.mkdtemp(dir=self.root))
        apt_dir = root / "apt"
        source = apt_dir / source_file
        source.parent.mkdir(parents=True)
        if source_file.endswith(".sources"):
            source.write_text(f"Types: deb\nURIs: {mirror}\nSuites: resolute\nComponents: main\n")
        else:
            source.write_text(f"{mirror}\nhttp://security.ubuntu.com/ubuntu\n")
        if source_file == "apt-mirrors.txt":
            reference = apt_dir / "sources.list.d/ubuntu.sources"
            reference.parent.mkdir()
            reference.write_text(f"Types: deb\nURIs: mirror+file:{source}\nSuites: resolute\n")
        # Redirect /etc into a fixture; keep real sed so URL changes are tested.
        (root / "installer.sh").write_text(SCRIPT.read_text().replace("/etc/apt", str(apt_dir)))
        shutil.copy(SCRIPT.with_name("document-tools.packages"), root)
        shim = root / "shim"
        shim.write_text("""#!/usr/bin/env bash
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
    fi
    cache=$UNCACHED
    keep=false
    for arg in "$@"; do
      case "$arg" in
        Dir::Cache::archives=*) cache=${arg#*=} ;;
        APT::Keep-Downloaded-Packages=true) keep=true ;;
      esac
    done
    mkdir -p "$cache"
    if [[ " $* " == *' --download-only '* ]]; then
      if [[ -f "$cache/tools.deb" ]]; then
        echo cache-hit >> "$TRACE"
      else
        echo network-download >> "$TRACE"
        touch "$cache/tools.deb"
      fi
    elif [[ " $* " == *' --no-download '* ]] && ! "$keep"; then
      rm -f "$cache/tools.deb"
    fi ;;
esac
""")
        shim.chmod(0o755)
        for name in ("apt-get", "timeout"):
            (root / name).symlink_to(shim)
        env = {
            **os.environ,
            "PATH": f"{root}:{os.environ['PATH']}",
            "TRACE": str(root / "trace"),
            "COUNT": str(root / "count"),
            "FAILURES": str(failures),
            "FAIL_AT": fail_at,
            "UNCACHED": str(root / "uncached"),
        }
        return root, env

    def run_installer(
        self, root: Path, env: dict[str, str]
    ) -> tuple[subprocess.CompletedProcess[str], list[str]]:
        bash = shutil.which("bash")
        self.assertIsNotNone(bash)
        result = subprocess.run(
            [str(bash), str(root / "installer.sh"), str(root / "archives")],
            env=env,
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
        return result, (root / "trace").read_text().splitlines()

    def test_success_installs_only_after_bounded_download(self) -> None:
        result, trace = self.run_installer(*self.fixture())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum(line.startswith("apt-get") and "--download-only" in line for line in trace), 1)
        self.assertTrue(any(line.startswith("timeout") and "30s" in line for line in trace))
        self.assertTrue(any(line.startswith("timeout") and "90s" in line for line in trace))
        self.assertIn("--no-download", trace[-1])
        self.assertIn("libreoffice-writer-nogui", trace[-1])

    def test_network_failure_retries_before_installing(self) -> None:
        for fail_at in ("download", "update"):
            with self.subTest(fail_at=fail_at):
                result, trace = self.run_installer(*self.fixture(1, fail_at))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("::warning::", result.stdout)
                downloads = sum(line.startswith("apt-get") and "--download-only" in line for line in trace)
                self.assertEqual(downloads, 2 if fail_at == "download" else 1)
                self.assertIn("--no-download", trace[-1])

    def test_persistent_network_failure_never_configures_packages(self) -> None:
        for fail_at in ("download", "update"):
            with self.subTest(fail_at=fail_at):
                result, trace = self.run_installer(*self.fixture(2, fail_at))
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any("--no-download" in line for line in trace))

    def test_configuration_failure_is_not_hidden_or_retried(self) -> None:
        result, trace = self.run_installer(*self.fixture(1, "configure"))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(sum(line.startswith("apt-get") and "--no-download" in line for line in trace), 1)

    def test_retry_switches_direct_sources_and_mirror_lists(self) -> None:
        for source_file in ("apt-mirrors.txt", "sources.list", "sources.list.d/ubuntu.sources"):
            with self.subTest(source_file=source_file):
                root, env = self.fixture(1, source_file=source_file)
                result, _ = self.run_installer(root, env)
                self.assertEqual(result.returncode, 0, result.stderr)
                content = (root / "apt" / source_file).read_text()
                self.assertIn("https://archive.ubuntu.com/ubuntu\n", content)
                self.assertNotIn("azure.archive.ubuntu.com", content)
                if source_file == "apt-mirrors.txt":
                    self.assertIn("https://security.ubuntu.com/ubuntu\n", content)
                    reference = (root / "apt/sources.list.d/ubuntu.sources").read_text()
                    self.assertIn("mirror+file:", reference)

    def test_renamed_runner_archive_is_supported(self) -> None:
        root, env = self.fixture(1, mirror="https://renamed-runner.archive.ubuntu.com/ubuntu")
        result, _ = self.run_installer(root, env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("https://archive.ubuntu.com/ubuntu\n", (root / "apt/apt-mirrors.txt").read_text())

    def test_retry_upgrades_http_main_archive_and_security(self) -> None:
        root, env = self.fixture(1, mirror="http://archive.ubuntu.com/ubuntu")
        result, _ = self.run_installer(root, env)
        self.assertEqual(result.returncode, 0, result.stderr)
        content = (root / "apt/apt-mirrors.txt").read_text()
        self.assertIn("https://archive.ubuntu.com/ubuntu", content)
        self.assertIn("https://security.ubuntu.com/ubuntu", content)
        self.assertNotIn("http://", content)

    def test_unknown_or_already_main_mirror_warns_when_unchanged(self) -> None:
        for mirror in ("https://mirror.example.org/ubuntu", "https://archive.ubuntu.com/ubuntu"):
            with self.subTest(mirror=mirror):
                root, env = self.fixture(1, source_file="sources.list.d/ubuntu.sources", mirror=mirror)
                result, _ = self.run_installer(root, env)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("::warning::No supported Ubuntu mirror URL was changed", result.stdout)
                self.assertIn(mirror, (root / "apt/sources.list.d/ubuntu.sources").read_text())

    def test_second_install_reuses_and_retains_downloaded_packages(self) -> None:
        root, env = self.fixture()
        first, _ = self.run_installer(root, env)
        second, trace = self.run_installer(root, env)
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(trace.count("network-download"), 1)
        self.assertEqual(trace.count("cache-hit"), 1)
        self.assertTrue((root / "archives/tools.deb").is_file())


if __name__ == "__main__":
    unittest.main()
