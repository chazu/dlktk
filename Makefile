BINARY := dlktk
PKG := ./cmd/dlktk

# install location: GOBIN if set, else GOPATH/bin
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif
INSTALL_PATH := $(GOBIN)/$(BINARY)

# ad-hoc code signing (macOS only; no-op elsewhere)
UNAME := $(shell uname -s)
ifeq ($(UNAME),Darwin)
CODESIGN := codesign --force --sign -
else
CODESIGN := true
endif

SKILL_SRC := skills/dlktk-dialectic
SKILL_DEST := $(HOME)/.claude/skills/dlktk-dialectic

.PHONY: build install install-skill sign verify clean test

## build: compile + ad-hoc sign the local binary
build:
	go build -o $(BINARY) $(PKG)
	$(CODESIGN) $(BINARY)

## install: go install + ad-hoc sign into $(GOBIN)
install:
	go install $(PKG)
	$(CODESIGN) $(INSTALL_PATH)
	@echo "installed + ad-hoc signed: $(INSTALL_PATH)"

## sign: re-sign the installed binary (ad-hoc)
sign:
	$(CODESIGN) $(INSTALL_PATH)

## verify: check the installed binary's signature
verify:
	codesign --verify --verbose $(INSTALL_PATH)

## install-skill: sync the repo dialectic skill into ~/.claude/skills. A
## convenience only — the guarantee is the skill's version handshake at session
## start (it checks the running contract), not this copy.
install-skill:
	mkdir -p $(SKILL_DEST)
	cp -R $(SKILL_SRC)/ $(SKILL_DEST)/
	@echo "synced skill: $(SKILL_DEST)"

## test: run the test suite
test:
	go test ./...

## clean: remove the local build artifact
clean:
	rm -f $(BINARY)
