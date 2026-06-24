# HC Webhook Backend — Design Spec

**Date:** 2026-06-24  
**Project:** HealthConnect (working name: `hcw`)  
**Source app:** [health-connect-webhook](https://github.com/ya-breeze/health-connect-webhook) (fork of mcnaveen/health-connect-webhook)

---

## Overview

A Go backend that receives health data POSTed by the HC Webhook Android app, stores it in SQLite, exposes a REST API and a Next.js dashboard for browsing data, and provides an HTTP-based MCP server for LLM access. Designed for home-server use with multiple family members, no internet-facing concerns.

---

## Architecture

Single Go binary (`hcw`) + Next.js frontend + nginx, following the same docker-compose pattern as GeekBudgetBE and KinCart.

```
Android (HC Webhook app)
    │  POST /webhook/{username}
    ▼
nginx  ──►  Go backend (hcw server)
                ├── Webhook receiver      POST /webhook/{username}
                ├── Auth endpoints        POST /api/auth/*
                ├── REST API              GET  /api/data/*
                └── MCP endpoint               /mcp
            │
            SQLite (GORM)
            │
nginx  ──►  Next.js frontend  (static, served by nginx)
```

**Stack:**
- Go 1.26, GORM + SQLite, Cobra + Viper
- `github.com/ya-breeze/kin-core` — Family/User models, JWT auth, cookies, token blacklist
- `github.com/modelcontextprotocol/go-sdk` — MCP HTTP streamable transport
- Next.js (App Router), Tailwind CSS, Recharts
- nginx reverse proxy

---

## Repository Structure

```
HealthConnect/
  Makefile
  docker-compose.yml
  docker-compose.wip.yml
  backend/
    Dockerfile
    cmd/
      main.go                    # Cobra root
      commands/
        cmdserver.go             # "server" subcommand
        cmdmcpconfig.go          # "mcp-config" subcommand
    pkg/
      config/                    # Viper env var bindings
      database/                  # GORM+SQLite, migrations, storage interface
      server/                    # HTTP handlers: webhook, REST, MCP
      mcpserver/                 # MCP tool definitions and instructions
  frontend/
    Dockerfile
    (Next.js app)
  nginx/
    Dockerfile
    nginx.conf
  openspec/                      # OpenSpec change tracking
```

**CLI commands:**
```bash
hcw server       # starts HTTP server (webhook + REST + MCP + serves frontend)
hcw mcp-config   # writes .mcp.json for Claude Desktop / Claude Code
```

---

## Authentication & Multi-User

Uses `kin-core` for auth:
- **`Family`** — tenant group (household)
- **`User`** — individual within a family, has `username` + `password_hash`
- JWT access token in `kin_access` cookie, refresh token in `kin_refresh` cookie
- Token blacklist for logout

**Seeding:** `HCW_SEED_USERS=FamilyName:username:password,...`  
Multiple users in the same family share the same `FamilyName`. Created on startup if they don't exist.

**Webhook endpoint is unauthenticated** — identified by username in URL path only.  
**REST API and frontend are JWT-protected.**  
**MCP endpoint is unauthenticated** — expected to be on trusted home LAN.

Family isolation: REST API queries are scoped to `family_id` from JWT claims. A user can query any member of their family via `?user=` param but cannot cross family boundaries.

---

## Data Model

### From kin-core (auto-migrated)
- `families` — `id`, `name`
- `users` — `id`, `username`, `password_hash`, `family_id`
- `blacklisted_tokens`, `refresh_tokens`

### `webhook_payloads` — raw audit log
Embeds `TenantModel` (`id`, `family_id`, `created_at`, `updated_at`, `deleted_at`).

| Column | Type | Notes |
|--------|------|-------|
| `user_id` | UUID | FK → users |
| `received_at` | DATETIME | when POST arrived |
| `app_version` | TEXT | from payload |
| `payload_ts` | DATETIME | `timestamp` field from payload |
| `raw` | TEXT | full original JSON blob |

### Per-type health tables
Each embeds `TenantModel` + `user_id UUID` + `source_payload_id UUID` (FK → webhook_payloads).  
Unique index on `(user_id, <time key(s)>)` for deduplication.

| Table | Columns (beyond common fields) |
|-------|-------------------------------|
| `steps` | `start_time`, `end_time`, `count INT` |
| `heart_rate` | `time`, `bpm INT` |
| `heart_rate_variability` | `time`, `rmssd_millis REAL` |
| `sleep` | `start_time` (derived: session_end_time − duration_seconds), `session_end_time`, `duration_seconds INT` |
| `sleep_stages` | `sleep_id` FK, `stage TEXT`, `start_time`, `end_time`, `duration_seconds INT` |
| `blood_pressure` | `time`, `systolic REAL`, `diastolic REAL` |
| `distance` | `start_time`, `end_time`, `meters REAL` |
| `active_calories` | `start_time`, `end_time`, `calories REAL` |
| `total_calories` | `start_time`, `end_time`, `calories REAL` |
| `weight` | `time`, `kilograms REAL` |
| `height` | `time`, `meters REAL` |
| `blood_glucose` | `time`, `mmol_per_liter REAL` |
| `oxygen_saturation` | `time`, `percentage REAL` |
| `body_temperature` | `time`, `celsius REAL` |
| `skin_temperature` | `time`, `delta_celsius REAL`, `baseline_celsius REAL` (nullable), `measurement_location INT` |
| `respiratory_rate` | `time`, `rate REAL` |
| `resting_heart_rate` | `time`, `bpm INT` |
| `exercise` | `start_time`, `end_time`, `duration_seconds INT`, `exercise_type TEXT`, `distance_meters REAL?`, `steps INT?`, `avg_cadence_spm REAL?`, `max_cadence_spm REAL?`, `stride_length_m REAL?` |
| `hydration` | `start_time`, `end_time`, `liters REAL` |
| `nutrition` | `start_time`, `end_time`, `calories REAL?`, `protein_grams REAL?`, `carbs_grams REAL?`, `fat_grams REAL?`, `sugar_grams REAL?`, `sodium_grams REAL?`, `dietary_fiber_grams REAL?`, `name TEXT?` |
| `basal_metabolic_rate` | `time`, `watts REAL` |
| `body_fat` | `time`, `percentage REAL` |
| `lean_body_mass` | `time`, `kilograms REAL` |
| `vo2_max` | `time`, `ml_per_kg_per_min REAL` |
| `bone_mass` | `time`, `kilograms REAL` |

**Deduplication:** on ingest, skip records where `(user_id, <time key>)` already exists.  
**Safety net:** `webhook_payloads.raw` preserves every original POST in full.  
**Sleep `start_time`:** derived as `session_end_time − duration_seconds`. Stages stored in `sleep_stages` linked by `sleep_id`.

---

## API

### Webhook receiver (unauthenticated)
```
POST /webhook/{username}
```
- Looks up user by name → `404` if unknown
- Stores `webhook_payloads` row
- Fans out into all present type tables
- Deduplicates per-record
- Returns `204`

### Auth (kin-core, unauthenticated)
```
POST /api/auth/login
POST /api/auth/logout
POST /api/auth/refresh
```

### REST API (JWT-protected)
```
GET /api/users/me
GET /api/data/{type}?from=&to=&user=     # query any type by time range
GET /api/data/summary?from=&to=&user=    # daily aggregates: total steps, avg HR, sleep duration
```
`{type}` is any of the 24 type names (e.g. `steps`, `sleep`, `heart_rate`).  
`user=` defaults to caller; must be in same family.

### MCP endpoint (unauthenticated)
```
/mcp   — HTTP streamable transport
```
Tools:
- `query_{type}(user, from, to)` — returns records for a given type and user
- `summary(user, from, to)` — daily aggregated summary
- `list_users()` — lists all users visible to the MCP client

`hcw mcp-config` generates `.mcp.json` pointing at `/mcp` for Claude Desktop / Claude Code.

---

## Frontend (Next.js)

Read-only dashboard. No data entry — all data arrives via webhook.

**Pages:**
- `/login` — username + password, sets JWT cookies
- `/` — dashboard: daily summary cards (steps, last sleep, current HR, HRV trend), family member switcher
- `/data/{type}` — time-series chart + data table for a single type, date range picker

**Stack:** Next.js App Router, Tailwind CSS, Recharts.  
Multi-stage Docker build; static export served by nginx.

---

## Deployment

**docker-compose services:** `backend`, `frontend`, `nginx` (no hardcoded `container_name`).

**Key env vars:**
```
HCW_PORT=8080
HCW_DBPATH=/data/hcw.db
HCW_SEED_USERS=MyFamily:alice:pass1,MyFamily:bob:pass2
HCW_JWT_SECRET=...
HCW_COOKIE_SECURE=true
HCW_BACKUP_PATH=/data/backups
HCW_BACKUP_INTERVAL=24h
HCW_BACKUP_MAX_COUNT=10
NGINX_HTTP_PORT=8888
NGINX_HTTPS_PORT=9888
```

**Port allocation:**
- WIP: `8888 / 9888` (next free after portfolio-analysis-wip at 8887/9887)
- Production: TBD when first deployed (next free prod port after kincart at 8883)

**data.json** entries: `deployments.hcw-wip` and `sync.hcw`.

---

## Out of Scope (v1)

- Data entry / editing via UI
- Push notifications / alerts (e.g. "HR above threshold")
- Charts comparing multiple users on the same graph
- Data export / import
- Frontend for displaying sleep stages breakdown (stored in DB, but UI shows duration only)
