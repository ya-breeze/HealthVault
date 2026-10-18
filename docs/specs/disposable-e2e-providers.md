# Local providers for disposable E2E

Idea: https://ideaforge.ikoro.in/idea/955

## Why

The existing HealthVault browser suite runs against a deployed backend. Its temporary test stack has a fresh database but currently cannot satisfy the suite: the profile seeds the wrong users, several assertions inherit health records from WIP, weather enrichment requires an external service, and food analysis may reach a billed LLM. E2E must be repeatable without external calls or WIP data.

## How

Add an explicit backend-only disposable E2E mode. In that mode, select deterministic local implementations of the existing weather and vision provider interfaces, even if an OpenAI key is present. Keep the ordinary runtime unchanged. The operator profile seeds the suite's users and webhook token. A disposable-only Playwright setup posts a small synthetic health history through the existing webhook. The profile retains the internal-only network and runs every existing Playwright spec. Declare that full suite as HealthVault's post-publication E2E gate and prohibit focused `test.only` cases in disposable mode. Do not add new browser scenarios, a public mock endpoint, or outbound network access. Do not change production or WIP stacks.

## Validation Commands

- `make test-backend`
- `make lint`
- Run the existing Playwright suite through the disposable test-environment profile; verify report, zero external calls, and resource cleanup.

### Task 1: Use local providers in disposable runs

- [x] Select deterministic local weather and vision providers only when `HCW_DISPOSABLE_E2E` is set.
- [x] Keep provider outputs sufficient for the existing E2E assertions without calling an external service.
- [x] Cover the mode choice and provider behavior with focused backend tests.
- [x] Mark completed.
