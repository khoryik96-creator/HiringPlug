import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]


class CodespaceUpdateTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        scripts = self.root / ".devcontainer"
        scripts.mkdir()
        shutil.copy(ROOT / ".devcontainer" / "update.sh", scripts / "update.sh")
        (scripts / "start.sh").write_text('printf "start\n" >> "$UPDATE_TEST_TRACE"\n')

        git = self.root / "git"
        git.write_text(
            '#!/bin/bash\n'
            'printf "%s\n" "$*" >> "$UPDATE_TEST_TRACE"\n'
            'case "$*" in\n'
            '  "branch --show-current") printf "%s\n" "$UPDATE_TEST_BRANCH" ;;\n'
            '  "pull --ff-only origin main") exit "$UPDATE_TEST_PULL_STATUS" ;;\n'
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
