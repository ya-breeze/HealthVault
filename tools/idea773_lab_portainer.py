"""Build and run the synthetic HealthVault readiness lab in a new Portainer stack.

This runner never updates or deletes a stack. It uses only committed files from
the current clean revision and an in-memory Compose transform with /data on tmpfs.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import re
import subprocess
import sys
import tarfile
import time
from pathlib import Path, PurePosixPath
from typing import Any

import yaml

PORTAINER_HELPER = Path("/data/Useful/ai/truenas")
DEFAULT_ENDPOINT = 3
STACK_PREFIX = "idea-773-healthvault-smoke"
EXPECTED_TMPFS = {"/data": "size=536870912"}
FORBIDDEN_SERVICE_KEYS = {
    "network_mode", "privileged", "pid", "ipc", "uts", "devices",
    "volumes_from", "env_file", "secrets", "cap_add", "userns_mode",
    "cgroupns", "runtime", "extra_hosts", "dns", "configs",
    "device_cgroup_rules",
}
SENSITIVE_PARTS = re.compile(
    r"(^|[._-])(secret|credentials?|tokens?|passwords?|private|id_rsa)([._-]|$)", re.IGNORECASE
)
SENSITIVE_SUFFIXES = {".pem", ".key", ".p12", ".pfx", ".db", ".sqlite", ".sqlite3"}
SOURCE_SUFFIXES = {".go", ".ts", ".tsx", ".js", ".jsx", ".css", ".scss", ".html"}
TEMPLATE_TOP_KEYS = {"name", "services", "networks", "volumes"}
TRANSFORMED_TOP_KEYS = {"name", "services", "networks"}
TEMPLATE_SERVICE_KEYS = {
    "backend": {"image", "build", "restart", "environment", "networks", "healthcheck", "volumes"},
    "nginx": {"image", "build", "restart", "depends_on", "networks", "healthcheck"},
    "lab-smoke": {"image", "depends_on", "restart", "entrypoint", "command", "networks", "profiles"},
}
TRANSFORMED_SERVICE_KEYS = {
    "backend": {"image", "restart", "environment", "networks", "healthcheck", "tmpfs"},
    "nginx": {"image", "restart", "depends_on", "networks", "healthcheck"},
    "lab-smoke": {"image", "depends_on", "restart", "entrypoint", "command", "networks"},
}
SMOKE_COMMAND_SHA256 = "5b3137017c1e1719e6c49fde0339518e9d3ce6073626778f27bc4a75c6bf17ad"


class LabError(RuntimeError):
    pass


def _git(repo: Path, *args: str) -> bytes:
    return subprocess.check_output(["git", "-C", str(repo), *args])


def revision(repo: Path) -> str:
    return _git(repo, "rev-parse", "--short=7", "HEAD").decode().strip()


def stack_name(repo: Path) -> str:
    return f"{STACK_PREFIX}-{revision(repo)}"


def assert_clean(repo: Path) -> None:
    branch = _git(repo, "branch", "--show-current").decode().strip()
    if branch != "feature/idea-773-backend-readiness":
        raise LabError("run is restricted to feature/idea-773-backend-readiness")
    if _git(repo, "status", "--porcelain", "--untracked-files=all").strip():
        raise LabError("run requires a clean worktree; commit or remove local changes first")


def is_safe_context_path(path: str) -> bool:
    p = PurePosixPath(path)
    if p.is_absolute() or ".." in p.parts:
        return False
    if any(part.lower().startswith(".env") for part in p.parts):
        return False
    if p.suffix.lower() not in SOURCE_SUFFIXES and any(SENSITIVE_PARTS.search(part) for part in p.parts):
        return False
    return p.suffix.lower() not in SENSITIVE_SUFFIXES


def tracked_paths(repo: Path) -> list[str]:
    paths = _git(repo, "ls-files", "-z", "--", "backend", "frontend", "nginx").decode().split("\0")
    return [p for p in paths if p]


def contexts(repo: Path) -> dict[str, list[str]]:
    all_paths = tracked_paths(repo)
    backend = [p for p in all_paths if p.startswith("backend/") and is_safe_context_path(p)]
    frontend = [
        p for p in all_paths
        if (p.startswith("frontend/") and p not in {
            "frontend/.gitignore", "frontend/README.md", "frontend/AGENTS.md",
            "frontend/CLAUDE.md", "frontend/Dockerfile",
        } and is_safe_context_path(p))
    ]
    nginx = ["nginx/Dockerfile", "nginx/nginx.conf"]
    if not set(nginx).issubset(all_paths):
        raise LabError("tracked Nginx Dockerfile or nginx.conf is missing")
    result = {"backend": sorted(backend), "nginx": sorted(frontend + nginx)}
    if not result["backend"] or not result["nginx"]:
        raise LabError("an allowlisted build context is empty")
    for paths in result.values():
        for path in paths:
            if not is_safe_context_path(path):
                raise LabError(f"unsafe path reached build allowlist: {path}")
            if (repo / path).is_symlink() or not (repo / path).is_file():
                raise LabError(f"allowlisted source is missing or not a regular file: {path}")
    return result


def make_tar(repo: Path, paths: list[str], strip_prefix: str = "") -> bytes:
    """Create a small tar in memory containing only the listed tracked files."""
    buf = io.BytesIO()
    tracked = set(tracked_paths(repo))
    with tarfile.open(fileobj=buf, mode="w") as tar:
        for path in paths:
            if path not in tracked or not is_safe_context_path(path):
                raise LabError(f"build tar path is not an allowlisted tracked source: {path}")
            source = repo / path
            if strip_prefix and not path.startswith(strip_prefix):
                raise LabError(f"path is outside expected build context: {path}")
            archive_path = path[len(strip_prefix):] if strip_prefix else path
            info = tarfile.TarInfo(archive_path)
            info.size = source.stat().st_size
            info.mode = source.stat().st_mode & 0o777
            with source.open("rb") as fh:
                tar.addfile(info, fh)
    return buf.getvalue()


def transformed_compose(repo: Path, tag: str, name: str) -> str:
    compose = yaml.safe_load((repo / "docker-compose.lab.yml").read_text())
    services = compose.get("services") or {}
    if set(services) != {"backend", "nginx", "lab-smoke"}:
        raise LabError("lab Compose service set changed; review this runner before using it")
    _assert_template_safe(compose)
    for service in services.values():
        service.pop("build", None)
        service.pop("volumes", None)
    services["backend"]["image"] = f"healthvault-lab-backend:{tag}"
    services["nginx"]["image"] = f"healthvault-lab-nginx:{tag}"
    services["lab-smoke"]["image"] = f"healthvault-lab-backend:{tag}"
    services["backend"]["tmpfs"] = ["/data:size=536870912"]
    services["lab-smoke"].pop("profiles", None)
    compose.pop("volumes", None)
    compose["name"] = name
    _assert_compose_safe(compose)
    return yaml.safe_dump(compose, sort_keys=False)


def _assert_compose_safe(compose: dict[str, Any]) -> None:
    if set(compose) != TRANSFORMED_TOP_KEYS:
        raise LabError("transformed Compose has an unreviewed top-level key")
    if "volumes" in compose or "secrets" in compose or "configs" in compose or compose.get("services") is None:
        raise LabError("transformed Compose must have no persistent volumes")
    services = compose["services"]
    if set(services) != set(TRANSFORMED_SERVICE_KEYS):
        raise LabError("transformed Compose service set changed")
    for name, service in services.items():
        if set(service) != TRANSFORMED_SERVICE_KEYS[name]:
            raise LabError(f"service {name} has an unreviewed Compose key")
        if "ports" in service or "volumes" in service:
            raise LabError(f"service {name} publishes ports or mounts persistent/host data")
        if FORBIDDEN_SERVICE_KEYS.intersection(service):
            raise LabError(f"service {name} has a forbidden host/security setting")
        if service.get("networks") != ["lab"]:
            raise LabError(f"service {name} must attach only to the internal lab network")
    if services["lab-smoke"].get("profiles"):
        raise LabError("lab-smoke must run automatically in the one-shot stack")
    if services["backend"].get("tmpfs") != ["/data:size=536870912"]:
        raise LabError("only the reviewed backend /data tmpfs is allowed")
    if any("tmpfs" in service for name, service in services.items() if name != "backend"):
        raise LabError("tmpfs is allowed only for the backend /data directory")
    if compose.get("networks") != {"lab": {"internal": True}}:
        raise LabError("lab network must remain internal")


def _assert_template_safe(compose: dict[str, Any]) -> None:
    """Reject unexpected unsafe source keys; only the exact named lab volume is transformed away."""
    if set(compose) != TEMPLATE_TOP_KEYS:
        raise LabError("template has an unreviewed top-level key")
    if compose.get("volumes") != {"lab-data": None}:
        raise LabError("template volumes changed; review before allowing the lab transform")
    if compose.get("networks") != {"lab": {"internal": True}}:
        raise LabError("template must define only the internal lab network")
    services = compose["services"]
    if set(services) != set(TEMPLATE_SERVICE_KEYS):
        raise LabError("template service set changed")
    for name, service in services.items():
        if set(service) != TEMPLATE_SERVICE_KEYS[name]:
            raise LabError(f"template service {name} has an unreviewed Compose key")
        if "ports" in service or FORBIDDEN_SERVICE_KEYS.intersection(service):
            raise LabError(f"template service {name} has an unsafe host/security setting")
        if service.get("networks") != ["lab"]:
            raise LabError(f"template service {name} must attach only to lab")
        if name == "backend":
            if service.get("volumes") != ["lab-data:/data"]:
                raise LabError("template backend volume changed; review before transforming")
            env = service.get("environment") or {}
            if not isinstance(env, dict):
                raise LabError("lab backend environment must remain a mapping")
            expected = {
                "HCW_PORT": "8080",
                "HCW_DBPATH": "/data/hcw.db",
                "HCW_SEED_USERS": "LabFamily:lab:lab-only-password",
                "HCW_JWT_SECRET": "healthvault-lab-only-not-a-secret",
                "HCW_COOKIE_SECURE": "false",
                "HCW_MCP_TOKEN": "",
                "HCW_CF_ACCESS_TEAM_DOMAIN": "",
                "HCW_CF_ACCESS_AUD": "",
                "HCW_CF_ACCESS_EMAIL_MAP": "",
                "HCW_UPLOADS_DIR": "/data/uploads",
                "HCW_USDA_DB_PATH": "/data/usda.db",
                "HCW_OFF_DB_PATH": "/data/off.db",
                "HCW_OPENAI_API_KEY": "",
            }
            if set(env) != set(expected):
                raise LabError("lab backend environment keys changed; review before running")
            for key, value in expected.items():
                if str(env.get(key, "")) != value:
                    raise LabError(f"synthetic lab environment changed at {key}; review before running")
        elif "volumes" in service:
            raise LabError(f"template service {name} must not have data mounts")
        if name == "lab-smoke":
            if service.get("profiles") != ["smoke"] or service.get("entrypoint") != ["/bin/sh", "-ec"]:
                raise LabError("smoke profile or entrypoint changed; review before running")
            command = service.get("command")
            if not isinstance(command, list) or len(command) != 1 or not isinstance(command[0], str):
                raise LabError("lab smoke command must remain the reviewed single shell script")
            digest = hashlib.sha256(command[0].encode()).hexdigest()
            if digest != SMOKE_COMMAND_SHA256:
                raise LabError("lab smoke command changed; review before running")


def _portainer():
    sys.path.insert(0, str(PORTAINER_HELPER))
    import portainer  # type: ignore[import-not-found]

    store = portainer.registry_store()
    settings = portainer.registry_domain(store, "portainer")
    token = portainer.portainer_get_token(store, settings)
    endpoint = getattr(portainer, "ENDPOINT_ID", None)
    if type(endpoint) is not int or endpoint != DEFAULT_ENDPOINT:
        raise LabError(f"Portainer helper endpoint must be reviewed endpoint {DEFAULT_ENDPOINT}")
    return portainer, settings["url"], token, endpoint


def _call(portainer: Any, url: str, token: str, method: str, path: str, payload: Any = None):
    status, body = portainer.portainer_call(url, token, method, path, payload)
    if status < 200 or status >= 300:
        raise LabError(f"Portainer {method} {path} failed with HTTP {status}")
    return body


def stack_collision(portainer: Any, url: str, token: str, name: str) -> bool:
    stacks = _call(portainer, url, token, "GET", "/api/stacks")
    return any(item.get("Name") == name for item in stacks)


def image_collision(portainer: Any, url: str, token: str, endpoint: int, tags: set[str]) -> bool:
    images = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/images/json?all=1")
    present = {tag for image in images for tag in (image.get("RepoTags") or []) if tag != "<none>:<none>"}
    return bool(tags & present)


def run_stack(repo: Path) -> None:
    assert_clean(repo)
    tag = revision(repo)
    name = stack_name(repo)
    allowlists = contexts(repo)
    compose = transformed_compose(repo, tag, name)
    portainer, url, token, endpoint = _portainer()
    if stack_collision(portainer, url, token, name):
        raise LabError(f"stack {name} already exists; refusing to overwrite or reuse it")
    image_tags = {f"healthvault-lab-backend:{tag}", f"healthvault-lab-nginx:{tag}"}
    if image_collision(portainer, url, token, endpoint, image_tags):
        raise LabError("one or more revision-specific image tags already exist; refusing to overwrite them")
    for context, image, dockerfile in (
        ("backend", f"healthvault-lab-backend:{tag}", "Dockerfile"),
        ("nginx", f"healthvault-lab-nginx:{tag}", "nginx/Dockerfile"),
    ):
        print(f"building {image} from {len(allowlists[context])} tracked allowlisted files")
        tar_bytes = make_tar(repo, allowlists[context], "backend/" if context == "backend" else "")
        portainer.portainer_build(url, token, endpoint, image, tar_bytes, dockerfile)
    payload = {"name": name, "stackFileContent": compose, "env": []}
    created = _call(portainer, url, token, "POST", f"/api/stacks/create/standalone/string?endpointId={endpoint}", payload)
    stack_id = created.get("Id", created.get("id"))
    if type(stack_id) is not int or created.get("Name") != name:
        raise LabError("Portainer did not confirm the exact new stack identity; inspect before proceeding")
    print(f"created stack id={stack_id} name={name}; stack and images are retained")
    smoke = wait_for_smoke(portainer, url, token, name, endpoint)
    print(f"lab-smoke exit={smoke['State']['ExitCode']}")
    print_container_logs(portainer, url, token, smoke["Id"], endpoint)
    if smoke["State"]["ExitCode"] != 0:
        raise LabError(f"lab smoke failed; retain stack for diagnosis with --stack-name {name}")
    verify_runtime(portainer, url, token, name, endpoint)
    stop_services(portainer, url, token, name, endpoint)
    print(f"smoke passed; application containers stopped; retained stack {name}")


def containers(portainer: Any, url: str, token: str, project: str, endpoint: int) -> list[dict[str, Any]]:
    rows = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/json?all=1")
    return [r for r in rows if (r.get("Labels") or {}).get("com.docker.compose.project") == project]


def wait_for_smoke(portainer: Any, url: str, token: str, project: str, endpoint: int, timeout: int = 360) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for row in containers(portainer, url, token, project, endpoint):
            labels = row.get("Labels") or {}
            if labels.get("com.docker.compose.service") == "lab-smoke" and row.get("State") == "exited":
                detail = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/json")
                return detail
        time.sleep(3)
    raise LabError(f"lab-smoke did not exit within {timeout}s; inspect retained stack {project}")


def print_container_logs(portainer: Any, url: str, token: str, cid: str, endpoint: int) -> None:
    status, data = portainer.portainer_raw_call(
        url, token, f"/api/endpoints/{endpoint}/docker/containers/{cid}/logs?stdout=1&stderr=1&tail=80"
    )
    if status != 200:
        raise LabError(f"could not read synthetic lab smoke logs (HTTP {status})")
    print(portainer.decode_docker_log_stream(data).decode("utf-8", errors="replace"))


def verify_runtime(portainer: Any, url: str, token: str, project: str, endpoint: int) -> None:
    rows = containers(portainer, url, token, project, endpoint)
    expected_services = {"backend", "nginx", "lab-smoke"}
    by_service = {}
    for row in rows:
        service = (row.get("Labels") or {}).get("com.docker.compose.service")
        if service not in expected_services or service in by_service:
            raise LabError("runtime contains an unexpected or duplicate service container")
        by_service[service] = row
    if len(rows) != 3 or set(by_service) != expected_services:
        raise LabError("runtime service set differs from the reviewed synthetic lab")
    expected_network_name = None
    for service, row in by_service.items():
        published = [p for p in row.get("Ports") or [] if p.get("PublicPort") or p.get("IP")]
        if published:
            raise LabError(f"runtime service {service} unexpectedly publishes a host port")
        detail = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/json")
        detail_labels = detail.get("Config", {}).get("Labels") or {}
        if detail_labels.get("com.docker.compose.project") != project or detail_labels.get("com.docker.compose.service") != service:
            raise LabError(f"runtime service {service} has mismatched inspect labels")
        host = detail.get("HostConfig") or {}
        if host.get("PortBindings") or host.get("Binds") or host.get("Mounts") or detail.get("Mounts"):
            raise LabError(f"runtime service {service} unexpectedly publishes ports or mounts data")
        runtime_network_mode = host.get("NetworkMode")
        network_settings = detail.get("NetworkSettings", {}).get("Networks") or {}
        if len(network_settings) != 1:
            raise LabError(f"runtime service {service} is not attached to exactly one lab network")
        network_name = next(iter(network_settings))
        if expected_network_name is None:
            expected_network_name = network_name
        if network_name != expected_network_name or runtime_network_mode != network_name or runtime_network_mode in {"host", "bridge", "none"}:
            raise LabError(f"runtime service {service} uses a host/default or mismatched network")
        if service == "backend" and host.get("Tmpfs") != EXPECTED_TMPFS:
            raise LabError("backend /data tmpfs does not match the reviewed 512 MiB lab setting")
        if service != "backend" and host.get("Tmpfs"):
            raise LabError(f"runtime service {service} has an unexpected tmpfs mount")
        expected_image = f"healthvault-lab-nginx:{revision(Path(__file__).resolve().parents[1])}"
        if service in {"backend", "lab-smoke"}:
            expected_image = f"healthvault-lab-backend:{revision(Path(__file__).resolve().parents[1])}"
        if detail.get("Config", {}).get("Image") != expected_image:
            raise LabError(f"runtime service {service} is not using the revision-pinned lab image")
        state = detail.get("State") or {}
        if service in {"backend", "nginx"}:
            if state.get("Status") != "running" or (state.get("Health") or {}).get("Status") != "healthy":
                raise LabError(f"runtime service {service} is not running and healthy")
        elif state.get("Status") != "exited" or state.get("ExitCode") != 0:
            raise LabError("runtime lab-smoke did not exit successfully")
    if not expected_network_name or not re.fullmatch(r"[a-z0-9_-]+", expected_network_name):
        raise LabError("runtime lab network name is malformed")
    network = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/networks/{expected_network_name}")
    if network.get("Internal") is not True:
        raise LabError("runtime lab network is not internal")


def stop_services(portainer: Any, url: str, token: str, project: str, endpoint: int) -> None:
    for row in containers(portainer, url, token, project, endpoint):
        labels = row.get("Labels") or {}
        service = labels.get("com.docker.compose.service")
        if service not in {"backend", "nginx"}:
            continue
        detail = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/json")
        exact = detail.get("Config", {}).get("Labels") or {}
        if exact.get("com.docker.compose.project") != project or exact.get("com.docker.compose.service") != service:
            raise LabError("container identity changed; refusing stop")
        if detail.get("State", {}).get("Running"):
            _call(portainer, url, token, "POST", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/stop?t=10")
            final = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/json")
            state = final.get("State", {})
            if state.get("Running") or state.get("Status") != "exited":
                raise LabError(f"failed to stop service {service}")
            print(f"stopped {service} ({row['Id'][:12]})")


def show_status(name: str, endpoint: int, show_logs: bool = False) -> None:
    if not re.fullmatch(r"idea-773-healthvault-smoke-[0-9a-f]{7}", name):
        raise LabError("read-only inspection accepts only the revision-scoped Idea 773 lab name")
    portainer, url, token, helper_endpoint = _portainer()
    if endpoint != helper_endpoint:
        raise LabError("requested endpoint differs from the reviewed Portainer helper endpoint")
    stack_list = _call(portainer, url, token, "GET", "/api/stacks")
    matches = [s for s in stack_list if s.get("Name") == name]
    if len(matches) != 1:
        raise LabError(f"expected exactly one retained stack named {name}; found {len(matches)}")
    print(f"stack id={matches[0].get('Id')} name={name} status={matches[0].get('Status')}")
    for row in containers(portainer, url, token, name, endpoint):
        labels = row.get("Labels") or {}
        detail = _call(portainer, url, token, "GET", f"/api/endpoints/{endpoint}/docker/containers/{row['Id']}/json")
        host = detail.get("HostConfig") or {}
        print(f"service={labels.get('com.docker.compose.service')} state={row.get('State')} ports={len(row.get('Ports') or [])} mounts={len(detail.get('Mounts') or [])} tmpfs={host.get('Tmpfs') or {}}")
        if show_logs and labels.get("com.docker.compose.service") == "lab-smoke":
            print_container_logs(portainer, url, token, row["Id"], endpoint)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "run", "status", "logs"))
    parser.add_argument("--stack-name")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    try:
        if args.action == "check":
            allowlists = contexts(repo)
            name = stack_name(repo)
            transformed_compose(repo, revision(repo), name)
            print(f"revision={revision(repo)} stack={name}")
            for context, paths in allowlists.items():
                print(f"{context}: {len(paths)} tracked files")
            print("transformed Compose: zero published ports, zero persistent/host mounts, backend /data tmpfs, internal network")
            return 0
        if args.action == "run" and args.stack_name is not None:
            raise LabError("run derives its own name from the current revision; --stack-name is read-only only")
        name = args.stack_name or stack_name(repo)
        if not re.fullmatch(r"idea-773-healthvault-smoke-[0-9a-f]{7}", name):
            raise LabError("stack name must be the revision-scoped Idea 773 lab name")
        if args.action == "run":
            run_stack(repo)
        elif args.action == "status":
            show_status(name, DEFAULT_ENDPOINT)
        elif args.action == "logs":
            show_status(name, DEFAULT_ENDPOINT, show_logs=True)
        return 0
    except (LabError, OSError, subprocess.CalledProcessError, yaml.YAMLError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
