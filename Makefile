.DEFAULT_GOAL := help
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.NOTPARALLEL:
VERSION ?= dev
export VERSION
.PHONY: help install lint test build artifact-smoke check dev stop clean
help:
	@echo 'make install: pinned tools; make check: CLI tests and all six release archives'
install:
	mise trust .mise.toml
	mise install
	mise exec -- go mod download
lint build artifact-smoke:
	mise exec -- bash scripts/profile-$@.sh
test:
	mise exec -- go test -race -count=1 ./...
check: lint build artifact-smoke test
dev stop:
	@echo '$@: unsupported: one-shot CLI has no automatic development service'
clean:
	mise exec -- python3 -c 'import shutil; [shutil.rmtree(p, ignore_errors=True) for p in ("bin", "dist", ".artifacts")]'
