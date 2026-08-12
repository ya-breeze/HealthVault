ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# SQLite FTS5 is a compile-time option in mattn/go-sqlite3. Without this tag,
# "CREATE VIRTUAL TABLE ... USING fts5" fails at runtime with "no such module: fts5",
# which the USDA food index depends on. Keep it on build, test and vet alike.
GO_TAGS := sqlite_fts5

.PHONY: all build test lint run-backend projected-specs

all: build

build:
	@cd $(ROOT_DIR)/backend/cmd && go build -tags $(GO_TAGS) -o ../bin/hcw .

test:
	@cd $(ROOT_DIR)/backend && go test -tags $(GO_TAGS) ./...

lint:
	@cd $(ROOT_DIR)/backend && go vet -tags $(GO_TAGS) ./...

projected-specs:
	@$(ROOT_DIR)scripts/generate-projected-specs.sh

run-backend: build
	@HCW_DBPATH=$(ROOT_DIR)hcw.db \
	HCW_JWT_SECRET=devsecret \
	HCW_COOKIE_SECURE=false \
	HCW_SEED_USERS="TestFamily:alice:pass1,TestFamily:bob:pass2" \
	$(ROOT_DIR)/backend/bin/hcw server
