## ADDED Requirements

### Requirement: Compose stack configurable via environment variables
The `docker-compose.yml` stack (backend + nginx) SHALL derive its host data directory and exposed ports entirely from environment variables, with no per-deployment values hardcoded in the compose file, so that multiple independent instances of the stack can run on the same host from the same compose file.

#### Scenario: Data directory is env-driven
- **WHEN** the compose stack is started with `HCW_DATA_PATH` set to a host directory
- **THEN** the backend container mounts that directory at `/data`, and all backend env vars that reference on-disk paths (`HCW_DBPATH`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`) resolve inside it

#### Scenario: Ports are env-driven
- **WHEN** the compose stack is started with `HCW_HTTP_PORT` and `HCW_HTTPS_PORT` set
- **THEN** nginx's host port bindings use those values instead of any fixed default, so two instances started with different values do not collide

#### Scenario: No stack identity is hardcoded
- **WHEN** the compose file is inspected
- **THEN** it declares no `container_name`, no hardcoded host bind path, and no hardcoded host port mapping — every instance-specific value comes from the environment
