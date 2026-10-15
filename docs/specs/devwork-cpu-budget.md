# Restrict development containers to the shared CPU set

## Why

A runtime CPU affinity update is lost when a development container is recreated.
Keep `hcw-wip` within the owner's shared logical CPU set `1,2,3`
when its Compose configuration is selected.

## How

Add the optional `compose.cpu-policy.yml` overlay for `backend`, `nginx`.
Set only `cpuset: "1,2,3"` on each service. Leave the base Compose files unchanged.
The operator selects this overlay only for the development stacks through Portainer's
persisted additional-file setting. Production stacks retain their existing configuration.
Existing CPU quotas, memory limits, mounts, application settings and images stay unchanged.
Image-build limits and transient E2E policy belong to the shared infrastructure work.
This change does not deploy a stack or switch its application revision.

## Validation Commands

```sh
git diff --check
python3 - <<'PYOVERLAY'
from pathlib import Path
import yaml
expected = {'backend': {'cpuset': '1,2,3'}, 'nginx': {'cpuset': '1,2,3'}}
assert yaml.safe_load(Path("compose.cpu-policy.yml").read_text()) == {"services": expected}
PYOVERLAY
make lint
```

### Task 1: Add the optional development CPU overlay
- [ ] Add `compose.cpu-policy.yml` for the exact development service set.
- [ ] Verify that the overlay changes only `cpuset` and preserves the base Compose files.
- [ ] Mark completed
