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

.PHONY: build install sign verify clean test

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

## test: run the test suite
test:
	go test ./...

## clean: remove the local build artifact
clean:
	rm -f $(BINARY)
