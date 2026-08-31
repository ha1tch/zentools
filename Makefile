# Makefile for zentools
#
# Wraps the conventions already established by .github/workflows/ and
# release.sh/syncver.sh -- this doesn't invent new ones. `make help` lists
# every target; `make` alone runs the same build+vet+test a PR check does.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

# The five commands this module ships, matching cmd/*/ exactly (and the
# `for cmd in ...` list in .github/workflows/release.yml -- keep both in
# sync if a command is ever added or removed).
CMDS := maketap totap loadtap tap2tzx zx

BIN_DIR  := bin
DIST_DIR := dist
VERSION  := $(shell cat VERSION 2>/dev/null || echo 0.0.0)
GOFLAGS  ?=

# Every target here is a command name or a workflow step, never a real
# output file this Makefile could compare mtimes against usefully.
.PHONY: all build install test test-race vet fmt fmt-check lint cover \
        clean release dist cross-build tidy verify help $(CMDS)

## all: build, vet, and test -- the same sequence a PR check runs (default target)
all: build vet test

## build: build all five commands into bin/
build: $(CMDS)

$(CMDS):
	@mkdir -p $(BIN_DIR)
	go build $(GOFLAGS) -o $(BIN_DIR)/$@ ./cmd/$@

## install: go install all five commands to $GOBIN (or $GOPATH/bin)
install:
	@for cmd in $(CMDS); do go install $(GOFLAGS) ./cmd/$$cmd; done

## test: go test ./... -race -count=1, matching .github/workflows/test.yml exactly
test:
	go test -race ./... -count=1

## test-race: alias for test (race is always on; kept for discoverability)
test-race: test

## vet: go vet ./...
vet:
	go vet ./...

## fmt: gofmt -w every .go file in the module
fmt:
	gofmt -w .

## fmt-check: fail if gofmt would change anything (CI-safe, no writes)
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

## lint: fmt-check + vet together
lint: fmt-check vet

## cover: go test -cover ./..., one line per package
cover:
	go test -cover ./...

## tidy: go mod tidy, then confirm it left go.mod/go.sum unchanged
tidy:
	go mod tidy
	@files="go.mod"; [ -f go.sum ] && files="go.mod go.sum"; \
	git diff --quiet -- $$files 2>/dev/null || \
		{ echo "$$files changed by tidy -- review and commit"; exit 1; }

## verify: go mod verify (checksum integrity of the module cache)
verify:
	go mod verify

## clean: remove build/dist output and checkpoint zips (never touches source)
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
	rm -f zentools-v*-checkpoint.zip

## release: run release.sh for VERSION (defaults to the VERSION file's own value)
##          usage: make release VERSION=0.9.0
release:
	./release.sh $(VERSION)

# Every target cross-build.yml covers, as a plain Make variable -- kept
# here rather than inside the recipe's own shell string, since a
# multi-line quoted shell string built from backslash-continued Makefile
# recipe lines is exactly the kind of thing that silently mis-joins.
CROSS_TARGETS := \
	linux/amd64 linux/386 linux/arm64 \
	darwin/amd64 darwin/arm64 \
	windows/amd64 windows/386 \
	freebsd/amd64 freebsd/arm64 freebsd/386 \
	openbsd/amd64 openbsd/arm64 \
	netbsd/amd64 netbsd/arm64 \
	dragonfly/amd64

## cross-build: build every command for every target cross-build.yml covers, into dist/
##              (cgo disabled throughout, matching CI -- zentools has no cgo to lose)
cross-build:
	@mkdir -p $(DIST_DIR)
	@for t in $(CROSS_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		stage="$(DIST_DIR)/zentools-$(VERSION)-$${os}-$${arch}"; \
		mkdir -p "$$stage"; \
		echo "  $$os/$$arch"; \
		for cmd in $(CMDS); do \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build -trimpath -o "$$stage/$$cmd$$ext" ./cmd/$$cmd || exit 1; \
		done; \
	done
	@echo "cross-build: $(DIST_DIR)/ populated for $(words $(CROSS_TARGETS)) targets"

## dist: alias for cross-build
dist: cross-build

## help: list every target with its one-line description
help:
	@grep -E '^## [a-zA-Z_-]+:' $(MAKEFILE_LIST) | sed 's/^## /  /' | sort
