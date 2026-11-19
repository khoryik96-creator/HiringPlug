import os
import shutil
import subprocess
import tempfile
import time
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]


class CodespaceUpdateTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name) / "checkout"
        scripts = self.root / ".devcontainer"
        scripts.mkdir(parents=True)
        shutil.copy(ROOT / ".devcontainer" / "update.sh", scripts / "update.sh")
        (scripts / "start.sh").write_text('printf "start\n" >> "$UPDATE_TEST_TRACE"\n')

        git = self.root / "git"
        git.write_text(
            '#!/bin/bash\n'
            'printf "%s\n" "$*" >> "$UPDATE_TEST_TRACE"\n'
            'case "$*" in\n'
            '  "branch --show-current") printf "%s\n" "$UPDATE_TEST_BRANCH" ;;\n'
            '  "pull --ff-only origin main") exit "$UPDATE_TEST_PULL_STATUS" ;;\n'
            '  "config user.email") echo owner@example.test ;;\n'
            '  "config user.name") echo Owner ;;\n'
            '  *) exit 99 ;;\n'
            'esac\n'
        )
        git.chmod(0o700)
        self.trace = self.root / "trace"

    def run_update(self, *, codespace="demo", branch="main", pull_status="0"):
        return subprocess.run(
            ["bash", str(self.root / ".devcontainer" / "update.sh")],
            env={
                **os.environ,
                "PATH": f"{self.root}:{os.environ['PATH']}",
                "CODESPACE_NAME": codespace,
                "UPDATE_TEST_BRANCH": branch,
                "UPDATE_TEST_PULL_STATUS": pull_status,
                "UPDATE_TEST_TRACE": str(self.trace),
            },
            capture_output=True,
            text=True,
            check=False,
            timeout=10,
        )

    def test_pulls_main_then_restarts_the_existing_codespace(self):
        result = self.run_update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.trace.read_text().splitlines(), ["branch --show-current", "pull --ff-only origin main", "start"])

    def test_stops_when_pull_fails(self):
        result = self.run_update(pull_status="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.trace.read_text().splitlines(), ["branch --show-current", "pull --ff-only origin main"])

    def test_leaves_feature_branches_alone(self):
        result = self.run_update(branch="feature")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.trace.read_text().splitlines(), ["branch --show-current"])

    def test_requires_a_codespace(self):
        result = self.run_update(codespace="")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.trace.exists())

    def test_update_waits_for_the_first_start_to_finish(self):
        secrets = self.root / "fixture.env"
        secrets.write_text("HP_PUBLIC_URL=http://localhost:3000\n")
        (self.root / "deploy").mkdir()
        (self.root / "deploy" / ".env").symlink_to(secrets)
        scripts = self.root / ".devcontainer"
        startup = (ROOT / ".devcontainer" / "start.sh").read_text()
        (scripts / "start.sh").write_text(startup.replace("/workspaces/.hiringplug-codespace.env", str(secrets)))
        docker = self.root / "docker"
        docker.write_text(
            '#!/bin/bash\n'
            'case "$*" in\n'
            '  info) exit 0 ;;\n'
            '  "compose -f deploy/docker-compose.yml up "*)\n'
            '    echo up >> "$UPDATE_TEST_TRACE"\n'
            '    until [[ -f release ]]; do sleep .025; done ;;\n'
            '  "compose -f deploy/docker-compose.yml exec "*)\n'
            '    if [[ -f created ]]; then echo 1; else echo 0; fi ;;\n'
            '  "compose -f deploy/docker-compose.yml run "*)\n'
            '    echo create >> "$UPDATE_TEST_TRACE"; touch created ;;\n'
            '  *) exit 99 ;;\n'
            'esac\n'
        )
        docker.chmod(0o700)
        env = {
            **os.environ, "PATH": f"{self.root}:{os.environ['PATH']}",
            "CODESPACE_NAME": "demo", "UPDATE_TEST_TRACE": str(self.trace),
            "UPDATE_TEST_BRANCH": "main", "UPDATE_TEST_PULL_STATUS": "0",
        }
        processes = []
        try:
            for script in ("start.sh", "update.sh"):
                processes.append(subprocess.Popen(["bash", str(scripts / script)], env=env, stdout=subprocess.DEVNULL))
                marker = "up" if script == "start.sh" else "branch --show-current"
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and (
                    not self.trace.exists() or marker not in self.trace.read_text().splitlines()
                ):
                    time.sleep(.025)
                self.assertIn(marker, self.trace.read_text().splitlines())
            self.assertNotIn("pull --ff-only origin main", self.trace.read_text().splitlines())
            (self.root / "release").touch()
            for process in processes:
                self.assertEqual(process.wait(timeout=5), 0)
            trace = self.trace.read_text().splitlines()
            self.assertEqual(trace.count("create"), 1)
            self.assertLess(trace.index("create"), trace.index("pull --ff-only origin main"))
        finally:
            (self.root / "release").touch()
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=5)
