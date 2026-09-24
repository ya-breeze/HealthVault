from __future__ import annotations

import copy
import io
import tarfile
import unittest
from pathlib import Path
from unittest import mock

import yaml

from tools import idea773_lab_portainer as lab

REPO = Path(__file__).resolve().parents[1]


class LabPortainerTests(unittest.TestCase):
    def test_sensitive_or_traversal_paths_are_rejected(self) -> None:
        for path in (
            "../secret.txt", "/etc/passwd", "backend/.env", "frontend/.env.local",
            "frontend/token.json", "backend/private.pem", "backend/data.sqlite",
        ):
            with self.subTest(path=path):
                self.assertFalse(lab.is_safe_context_path(path))
        self.assertTrue(lab.is_safe_context_path("backend/cmd/main.go"))

    def test_build_contexts_use_only_tracked_safe_project_paths(self) -> None:
        contexts = lab.contexts(REPO)
        self.assertIn("backend/go.mod", contexts["backend"])
        self.assertIn("backend/Dockerfile", contexts["backend"])
        self.assertIn("backend/pkg/server/access_token.go", contexts["backend"])
        self.assertIn("backend/pkg/server/access_token_test.go", contexts["backend"])
        self.assertIn("frontend/package-lock.json", contexts["nginx"])
        self.assertIn("nginx/Dockerfile", contexts["nginx"])
        self.assertIn("nginx/nginx.conf", contexts["nginx"])
        self.assertEqual(144, len(contexts["backend"]))
        self.assertEqual(96, len(contexts["nginx"]))
        for paths in contexts.values():
            self.assertTrue(all(lab.is_safe_context_path(path) for path in paths))
            self.assertTrue(all((REPO / path).is_file() for path in paths))

    def test_backend_tar_strips_repo_prefix_for_dockerfile_copy_paths(self) -> None:
        tar_bytes = lab.make_tar(REPO, ["backend/go.mod", "backend/go.sum"], "backend/")
        with tarfile.open(fileobj=io.BytesIO(tar_bytes)) as archive:
            self.assertEqual({"go.mod", "go.sum"}, set(archive.getnames()))
        with self.assertRaises(lab.LabError):
            lab.make_tar(REPO, ["frontend/.env.local"])

    def test_transformed_compose_is_internal_ephemeral_and_runs_smoke(self) -> None:
        raw = lab.transformed_compose(REPO, "abc1234", "idea-773-healthvault-smoke-abc1234")
        compose = yaml.safe_load(raw)
        services = compose["services"]
        self.assertNotIn("volumes", compose)
        self.assertEqual(["/data:size=536870912"], services["backend"]["tmpfs"])
        self.assertNotIn("ports", services["backend"])
        self.assertNotIn("volumes", services["backend"])
        self.assertNotIn("profiles", services["lab-smoke"])
        self.assertEqual("healthvault-lab-backend:abc1234", services["lab-smoke"]["image"])
        self.assertTrue(compose["networks"]["lab"]["internal"])
        for service in services.values():
            self.assertNotIn("ports", service)
            self.assertNotIn("volumes", service)

    def test_transformed_compose_rejects_host_and_privilege_escape_keys(self) -> None:
        compose = yaml.safe_load(lab.transformed_compose(REPO, "abc1234", "idea-773-healthvault-smoke-abc1234"))
        for key, value in (
            ("network_mode", "host"), ("privileged", True), ("pid", "host"),
            ("devices", ["/dev/null:/dev/null"]), ("volumes_from", ["other"]),
            ("env_file", [".env"]), ("secrets", ["prod"]), ("cap_add", ["SYS_ADMIN"]),
            ("external_links", ["other"]), ("security_opt", ["seccomp=unconfined"]),
        ):
            with self.subTest(key=key):
                changed = copy.deepcopy(compose)
                changed["services"]["backend"][key] = value
                with self.assertRaises(lab.LabError):
                    lab._assert_compose_safe(changed)

    def test_template_rejects_unknown_top_level_and_service_keys(self) -> None:
        template = yaml.safe_load((REPO / "docker-compose.lab.yml").read_text())
        template["services"]["backend"]["security_opt"] = ["seccomp=unconfined"]
        with self.assertRaises(lab.LabError):
            lab._assert_template_safe(template)

    def test_template_rejects_real_secret_and_home_url_in_environment(self) -> None:
        for key, value in (("HCW_OPENAI_API_KEY", "sk-not-a-real-key"), ("HCW_HOME_URL", "http://192.168.1.1")):
            with self.subTest(key=key):
                template = yaml.safe_load((REPO / "docker-compose.lab.yml").read_text())
                template["services"]["backend"]["environment"][key] = value
                with self.assertRaises(lab.LabError):
                    lab._assert_template_safe(template)

    def test_template_rejects_changed_smoke_command_or_profile(self) -> None:
        template = yaml.safe_load((REPO / "docker-compose.lab.yml").read_text())
        template["services"]["lab-smoke"]["profiles"] = ["manual"]
        with self.assertRaises(lab.LabError):
            lab._assert_template_safe(template)
        template = yaml.safe_load((REPO / "docker-compose.lab.yml").read_text())
        template["services"]["lab-smoke"]["command"][0] += "\necho unexpected"
        with self.assertRaises(lab.LabError):
            lab._assert_template_safe(template)
        template = yaml.safe_load((REPO / "docker-compose.lab.yml").read_text())
        template["external_links"] = ["host"]
        with self.assertRaises(lab.LabError):
            lab._assert_template_safe(template)

    def test_collision_is_detected_without_overwrite(self) -> None:
        with mock.patch.object(lab, "_call", return_value=[{"Name": "idea-773-healthvault-smoke-abc1234"}]) as call:
            self.assertTrue(lab.stack_collision(object(), "unused", "unused", "idea-773-healthvault-smoke-abc1234"))
            call.assert_called_once()

    def test_existing_image_tag_is_a_refused_collision(self) -> None:
        with mock.patch.object(lab, "_call", return_value=[{"RepoTags": ["healthvault-lab-backend:abc1234"]}]) as call:
            self.assertTrue(lab.image_collision(object(), "unused", "unused", 3, {"healthvault-lab-backend:abc1234"}))
            call.assert_called_once()

    def test_run_refuses_image_collision_before_build_or_stack_create(self) -> None:
        class FakePortainer:
            def __init__(self) -> None:
                self.calls: list[tuple[str, str]] = []
                self.builds = 0

            def portainer_call(self, _url, _token, method, path, _payload=None):
                self.calls.append((method, path))
                if path == "/api/stacks":
                    return 200, []
                if "/images/json" in path:
                    return 200, [{"RepoTags": ["healthvault-lab-backend:abc1234"]}]
                raise AssertionError(f"unexpected API call: {method} {path}")

            def portainer_build(self, *_args):
                self.builds += 1

        fake = FakePortainer()
        with (
            mock.patch.object(lab, "assert_clean"),
            mock.patch.object(lab, "revision", return_value="abc1234"),
            mock.patch.object(lab, "stack_name", return_value="idea-773-healthvault-smoke-abc1234"),
            mock.patch.object(lab, "_portainer", return_value=(fake, "unused", "unused", 3)),
            self.assertRaisesRegex(lab.LabError, "refusing to overwrite"),
        ):
            lab.run_stack(REPO)
        self.assertEqual(0, fake.builds)
        self.assertFalse(any("create/standalone" in path for _, path in fake.calls))

    def test_runtime_verifier_rejects_duplicate_service_rows(self) -> None:
        rows = [
            {"Id": "backend", "Labels": {"com.docker.compose.project": "idea-773-healthvault-smoke-abc1234", "com.docker.compose.service": "backend"}},
            {"Id": "nginx", "Labels": {"com.docker.compose.project": "idea-773-healthvault-smoke-abc1234", "com.docker.compose.service": "nginx"}},
            {"Id": "smoke", "Labels": {"com.docker.compose.project": "idea-773-healthvault-smoke-abc1234", "com.docker.compose.service": "lab-smoke"}},
            {"Id": "backend-copy", "Labels": {"com.docker.compose.project": "idea-773-healthvault-smoke-abc1234", "com.docker.compose.service": "backend"}},
        ]
        with (
            mock.patch.object(lab, "_call", return_value=rows),
            self.assertRaisesRegex(lab.LabError, "unexpected or duplicate"),
        ):
            lab.verify_runtime(object(), "unused", "unused", "idea-773-healthvault-smoke-abc1234", 3)

    def test_runtime_verifier_accepts_healthy_internal_ephemeral_lab(self) -> None:
        project = "idea-773-healthvault-smoke-abc1234"
        network = f"{project}_lab"
        services = ("backend", "nginx", "lab-smoke")
        rows = [
            {"Id": service, "State": "running", "Ports": [], "Labels": {"com.docker.compose.project": project, "com.docker.compose.service": service}}
            for service in services
        ]

        def call(_client, _url, _token, _method, path, _payload=None):
            if path.endswith("containers/json?all=1"):
                return rows
            if "/networks/" in path:
                return {"Internal": True}
            service = path.split("/containers/", 1)[1].split("/", 1)[0]
            host = {"NetworkMode": network, "PortBindings": None, "Binds": None, "Mounts": None, "Tmpfs": None}
            if service == "backend":
                host["Tmpfs"] = lab.EXPECTED_TMPFS
            return {
                "Config": {
                    "Image": f"healthvault-lab-{('backend' if service in {'backend', 'lab-smoke'} else 'nginx')}:abc1234",
                    "Labels": {"com.docker.compose.project": project, "com.docker.compose.service": service},
                },
                "State": (
                    {"Status": "exited", "ExitCode": 0}
                    if service == "lab-smoke"
                    else {"Status": "running", "Health": {"Status": "healthy"}}
                ),
                "HostConfig": host,
                "Mounts": [],
                "NetworkSettings": {"Networks": {network: {}}},
            }

        with (
            mock.patch.object(lab, "_call", side_effect=call),
            mock.patch.object(lab, "revision", return_value="abc1234"),
        ):
            lab.verify_runtime(object(), "unused", "unused", project, 3)

    def test_runtime_verifier_rejects_host_port_bindings_and_network(self) -> None:
        project = "idea-773-healthvault-smoke-abc1234"
        network = f"{project}_lab"
        rows = [
            {"Id": service, "State": "running", "Ports": [], "Labels": {"com.docker.compose.project": project, "com.docker.compose.service": service}}
            for service in ("backend", "nginx", "lab-smoke")
        ]

        def call(_client, _url, _token, _method, path, _payload=None):
            if path.endswith("containers/json?all=1"):
                return rows
            cid = path.split("/containers/", 1)[1].split("/", 1)[0]
            service = cid
            host = {"NetworkMode": network, "PortBindings": None, "Binds": None, "Mounts": None, "Tmpfs": None}
            if service == "backend":
                host["Tmpfs"] = lab.EXPECTED_TMPFS
                host["PortBindings"] = {"8080/tcp": [{"HostPort": "8080"}]}
            return {
                "Config": {
                    "Image": f"healthvault-lab-{('backend' if service in {'backend', 'lab-smoke'} else 'nginx')}:abc1234",
                    "Labels": {"com.docker.compose.project": project, "com.docker.compose.service": service},
                },
                "State": (
                    {"Status": "exited", "ExitCode": 0}
                    if service == "lab-smoke"
                    else {"Status": "running", "Health": {"Status": "healthy"}}
                ),
                "HostConfig": host,
                "Mounts": [],
                "NetworkSettings": {"Networks": {network: {}}},
            }

        with (
            mock.patch.object(lab, "_call", side_effect=call),
            mock.patch.object(lab, "revision", return_value="abc1234"),
            self.assertRaisesRegex(lab.LabError, "publishes ports or mounts data"),
        ):
            lab.verify_runtime(object(), "unused", "unused", project, 3)

        def host_network(*args, **kwargs):
            result = call(*args, **kwargs)
            if isinstance(result, dict):
                result["HostConfig"]["PortBindings"] = None
                result["HostConfig"]["NetworkMode"] = "host"
            return result

        with (
            mock.patch.object(lab, "_call", side_effect=host_network),
            mock.patch.object(lab, "revision", return_value="abc1234"),
            self.assertRaisesRegex(lab.LabError, "host/default or mismatched network"),
        ):
            lab.verify_runtime(object(), "unused", "unused", project, 3)


if __name__ == "__main__":
    unittest.main()
