ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: all build test lint run-backend

all: build

build:
	@cd $(ROOT_DIR)/backend/cmd && go build -o ../bin/hcw .

test:
	@cd $(ROOT_DIR)/backend && go test ./...

lint:
	@cd $(ROOT_DIR)/backend && go vet ./...

run-backend: build
	@HCW_DBPATH=$(ROOT_DIR)hcw.db \
	HCW_JWT_SECRET=devsecret \
	HCW_COOKIE_SECURE=false \
	HCW_SEED_USERS="TestFamily:alice:pass1,TestFamily:bob:pass2" \
	$(ROOT_DIR)/backend/bin/hcw server
