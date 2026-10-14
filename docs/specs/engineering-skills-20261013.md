# Configure repository domain docs

## Why

Agents need repository-specific domain guidance while the agent environment supplies tracker
and triage configuration.

## How

Keep the repository's own instructions and its single-context domain documentation.
Use repository-relative documents. Keep environment tracker and triage settings outside this
repository. Preserve existing instructions and operational boundaries. Where existing instructions
refer to an external environment rule or absolute filesystem location, use repository-local
configuration and runbooks for the relevant operational facts. Change documentation only.

## Validation Commands

git diff --check
git diff origin/main...HEAD -- AGENTS.md CLAUDE.md docs/agents/

### Task 1: Configure the repository
- [x] Keep the repository's own instructions and one relative domain-doc pointer.
- [x] Ensure no repository-level tracker or triage configuration files or sections remain.
- [x] Preserve single-context domain guidance and relevant operational boundaries.
- [x] Verify relative links, absence of environment coupling and Review Gate results.
- [x] Mark completed.
