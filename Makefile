ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# SQLite FTS5 is a compile-time option in mattn/go-sqlite3. Without this tag,
# "CREATE VIRTUAL TABLE ... USING fts5" fails at runtime with "no such module: fts5",
# which the USDA food index depends on. Keep it on build, test and vet alike.
GO_TAGS := sqlite_fts5

.PHONY: all build test test-backend test-frontend test-android test-e2e test-e2e-disposable test-workflows lint lint-android lint-e2e run-backend android-apk

DISPOSABLE_RUNNER ?= /data/Useful/ai/truenas/test_environment.py
DISPOSABLE_PROFILE ?= /data/Useful/ai/truenas/test-environment-profiles/healthvault-synthetic.json
DISPOSABLE_PORTAINER_URL ?= https://192.168.1.54:31015
DISPOSABLE_BIND_IP ?= 192.168.1.54
DISPOSABLE_HTTP_PORT ?=
DISPOSABLE_SESSION ?=
DISPOSABLE_OWNER ?= $(DISPOSABLE_SESSION)
DISPOSABLE_PLAYWRIGHT_IMAGE ?= mcr.microsoft.com/playwright:v1.61.1-noble@sha256:5b8f294aff9041b7191c34a4bab3ac270157a28774d4b0660e9743297b697e48

all: build

build:
	@cd $(ROOT_DIR)/backend/cmd && go build -tags $(GO_TAGS) -o ../bin/hcw .

# `test` runs every suite. The frontend's Vitest suite covers the weight-chart
# pure functions (projection, crossing, horizon, BMI, band conversion) whose
# edge cases are impractical to reach through Playwright; leaving it out of
# `make test` meant nothing ever ran it.
test: test-backend test-frontend test-android

test-backend:
	@cd $(ROOT_DIR)/backend && go test -tags $(GO_TAGS) ./...

test-frontend:
	@cd $(ROOT_DIR)/frontend && npm test --silent

# Requires Python 3 and PyYAML. This reads workflow files without starting jobs.
test-workflows:
	@cd $(ROOT_DIR) && python3 -m unittest discover -s tests/workflows -p 'test_*.py'

# android/README.md has the SDK setup this needs. Both android/local.properties
# and $ANDROID_HOME are absent in this environment, so this prints a visible
# skip notice rather than failing — `make test` stays green, but green here
# proves nothing about android/; see docs/adr/ADR-014.
test-android:
	@if [ -z "$$ANDROID_HOME" ] && [ ! -f "$(ROOT_DIR)android/local.properties" ]; then \
		echo "SKIP test-android: no Android SDK found (ANDROID_HOME unset, android/local.properties absent)"; \
	else \
		cd $(ROOT_DIR)android && ./gradlew testDebugUnitTest; \
	fi

# E2E_ARGS forwards flags to Playwright — `make test-e2e E2E_ARGS=--retries=0`
# is how a stability gate asks for first-attempt results. Without it the
# config's `retries: 1` applies silently, and a run that failed once and passed
# on retry reports as "1 flaky" rather than as the failure it is.
test-e2e: $(ROOT_DIR)e2e/node_modules/.install-stamp
	@cd $(ROOT_DIR)e2e && BASE_URL=$(or $(BASE_URL),http://192.168.1.54:8892) npx playwright test --reporter=line $(E2E_ARGS)

# Run the existing full suite in a private, synthetic stack. The runner supplies
# BASE_URL inside that stack, retains results, and cleans up without using WIP.
test-e2e-disposable:
	@test -n "$(DISPOSABLE_HTTP_PORT)" && test -n "$(DISPOSABLE_SESSION)" || { echo "Set DISPOSABLE_HTTP_PORT and DISPOSABLE_SESSION" >&2; exit 2; }
	python3 "$(DISPOSABLE_RUNNER)" run-checkout --checkout "$(ROOT_DIR)" \
		--profile "$(DISPOSABLE_PROFILE)" \
		--runs-dir /data/data/test-environment-runs \
		--artifact-root /data/data/test-environment-artifacts \
		--owner "$(DISPOSABLE_OWNER)" --session "$(DISPOSABLE_SESSION)" \
		--portainer-url "$(DISPOSABLE_PORTAINER_URL)" --endpoint-id 3 \
		--bind-ip "$(DISPOSABLE_BIND_IP)" --http-port "$(DISPOSABLE_HTTP_PORT)" \
		--timeout-seconds 5400 --expiry-seconds 7200 \
		--playwright-base-image "$(DISPOSABLE_PLAYWRIGHT_IMAGE)"

$(ROOT_DIR)e2e/node_modules/.install-stamp: $(ROOT_DIR)e2e/package-lock.json
	@cd $(ROOT_DIR)e2e && npm ci
	@touch $(ROOT_DIR)e2e/node_modules/.install-stamp

lint: lint-android lint-e2e
	@cd $(ROOT_DIR)/backend && go vet -tags $(GO_TAGS) ./...

# Same skip condition and reasoning as test-android above.
lint-android:
	@if [ -z "$$ANDROID_HOME" ] && [ ! -f "$(ROOT_DIR)android/local.properties" ]; then \
		echo "SKIP lint-android: no Android SDK found (ANDROID_HOME unset, android/local.properties absent)"; \
	else \
		cd $(ROOT_DIR)android && ./gradlew lintDebug; \
	fi

# Builds the debug APK the owner can still sideload by hand. Signed Internal
# delivery uses android-delivery.json and the shared local delivery service.
android-apk:
	@cd $(ROOT_DIR)android && ./gradlew assembleDebug

# This suite mutates a live stack (creates/deletes meals, rewrites settings,
# writes records against hcw-wip), so a typo here is a real edit, not a test
# failure someone shrugs off. Gated into `lint` rather than left advisory,
# because an advisory check that nobody has to pass is a check nobody reads.
lint-e2e: $(ROOT_DIR)e2e/node_modules/.install-stamp
	@cd $(ROOT_DIR)e2e && npm run typecheck --silent

run-backend: build
	@HCW_DBPATH=$(ROOT_DIR)hcw.db \
	HCW_JWT_SECRET=devsecret \
	HCW_COOKIE_SECURE=false \
	HCW_SEED_USERS="TestFamily:alice:pass1,TestFamily:bob:pass2" \
	$(ROOT_DIR)/backend/bin/hcw server
