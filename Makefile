ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# SQLite FTS5 is a compile-time option in mattn/go-sqlite3. Without this tag,
# "CREATE VIRTUAL TABLE ... USING fts5" fails at runtime with "no such module: fts5",
# which the USDA food index depends on. Keep it on build, test and vet alike.
GO_TAGS := sqlite_fts5

.PHONY: all build test test-backend test-frontend test-e2e lint lint-e2e run-backend

all: build

build:
	@cd $(ROOT_DIR)/backend/cmd && go build -tags $(GO_TAGS) -o ../bin/hcw .

# `test` runs both suites. The frontend's Vitest suite covers the weight-chart
# pure functions (projection, crossing, horizon, BMI, band conversion) whose
# edge cases are impractical to reach through Playwright; leaving it out of
# `make test` meant nothing ever ran it.
test: test-backend test-frontend

test-backend:
	@cd $(ROOT_DIR)/backend && go test -tags $(GO_TAGS) ./...

test-frontend:
	@cd $(ROOT_DIR)/frontend && npm test --silent

# E2E_ARGS forwards flags to Playwright — `make test-e2e E2E_ARGS=--retries=0`
# is how a stability gate asks for first-attempt results. Without it the
# config's `retries: 1` applies silently, and a run that failed once and passed
# on retry reports as "1 flaky" rather than as the failure it is.
test-e2e: $(ROOT_DIR)e2e/node_modules/.install-stamp
	@cd $(ROOT_DIR)e2e && BASE_URL=$(or $(BASE_URL),http://192.168.1.54:8892) npx playwright test --reporter=line $(E2E_ARGS)

$(ROOT_DIR)e2e/node_modules/.install-stamp: $(ROOT_DIR)e2e/package-lock.json
	@cd $(ROOT_DIR)e2e && npm ci
	@touch $(ROOT_DIR)e2e/node_modules/.install-stamp

# This suite mutates a live stack (creates/deletes meals, rewrites settings,
# writes records against hcw-wip), so a typo here is a real edit, not a test
# failure someone shrugs off. Gated into `lint` rather than left advisory,
# because an advisory check that nobody has to pass is a check nobody reads.
lint: lint-e2e
	@cd $(ROOT_DIR)/backend && go vet -tags $(GO_TAGS) ./...

lint-e2e: $(ROOT_DIR)e2e/node_modules/.install-stamp
	@cd $(ROOT_DIR)e2e && npm run typecheck --silent

run-backend: build
	@HCW_DBPATH=$(ROOT_DIR)hcw.db \
	HCW_JWT_SECRET=devsecret \
	HCW_COOKIE_SECURE=false \
	HCW_SEED_USERS="TestFamily:alice:pass1,TestFamily:bob:pass2" \
	$(ROOT_DIR)/backend/bin/hcw server
