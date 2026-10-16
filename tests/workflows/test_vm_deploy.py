"""Check the fixed trusted-host workflow without running deployment commands."""

from pathlib import Path
import shlex
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[2]


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        self.workflow = yaml.load(
            (ROOT / ".forgejo/workflows/vm-deploy.yml").read_text(),
            Loader=yaml.BaseLoader,
        )
        self.job = self.workflow["jobs"]["deploy"]

    def test_only_main_push_uses_dedicated_runner(self):
        self.assertEqual(self.workflow["on"], {"push": {"branches": ["main"]}})
        self.assertEqual(set(self.workflow["jobs"]), {"deploy"})
        self.assertEqual(self.job["runs-on"], "vm-deploy-trusted")
        self.assertEqual(self.job["timeout-minutes"], "45")
        self.assertEqual(
            self.job["if"],
            "forgejo.event_name == 'push' && forgejo.ref == 'refs/heads/main'",
        )

    def test_event_identity_is_explicit(self):
        self.assertEqual(self.job["env"], {
            "FORGEJO_ACTIONS": "true",
            "FORGEJO_EVENT_NAME": "${{ forgejo.event_name }}",
            "FORGEJO_REF": "${{ forgejo.ref }}",
            "FORGEJO_SHA": "${{ forgejo.sha }}",
            "FORGEJO_REPOSITORY": "${{ forgejo.repository }}",
        })

    def test_commands_never_execute_repository_hooks(self):
        steps = self.job["steps"]
        self.assertEqual(len(steps), 2)
        self.assertEqual(set(steps[0]), {"name", "run"})
        self.assertEqual(shlex.split(steps[0]["run"]), [
            "python3", "/opt/vm-deploy/ci.py", "healthvault",
        ])
        self.assertEqual(set(steps[1]), {"name", "if", "run"})
        self.assertEqual(steps[1]["if"], "failure() || cancelled()")
        self.assertEqual(shlex.split(steps[1]["run"]), [
            "python3", "/opt/vm-deploy/deploy.py", "healthvault", "$FORGEJO_SHA",
            "--config", "/etc/vm-deploy/healthvault.json",
            "--activation", "/etc/vm-deploy/healthvault.activation.json",
            "--result", "/var/lib/vm-deploy/healthvault/ci-finalizer.json",
            "--ci", "--notify-only",
        ])


if __name__ == "__main__":
    unittest.main()
