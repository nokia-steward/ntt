PROJECT := github.com/nokia/ntt

# Common GNU installation folders
DESTDIR    ?=
PREFIX     ?= /usr/local
BINDIR     ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR     ?= ${PREFIX}/share/man
DATADIR    ?= ${PREFIX}/share/ntt
ETCDIR     ?= /etc

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
ZSHINSTALLDIR=${PREFIX}/share/zsh/site-functions

# Commands
GO ?= go

# Go specific stuff
export GO111MODULE=off
export GOPROXY=https://proxy.golang.org

GO_BUILD=$(GO) build

# If GOPATH not specified, use one in the local directory
ifeq ($(GOPATH),)
export GOPATH := $(CURDIR)/_output
unexport GOBIN
endif
FIRST_GOPATH := $(firstword $(subst :, ,$(GOPATH)))
GOPKGDIR := $(FIRST_GOPATH)/src/$(PROJECT)
GOPKGBASEDIR ?= $(shell dirname "$(GOPKGDIR)")

GOBIN := $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(FIRST_GOPATH)/bin
endif


# Go module support: set `-mod=vendor` to force use the vendored sources
ifeq ($(shell $(GO) help mod >/dev/null 2>&1 && echo true), true)
	GO_BUILD=GO111MODULE=on $(GO) build -mod=vendor
endif


GIT_COMMIT ?= $(shell git rev-parse HEAD 2> /dev/null || true)
VERSION = $(shell git describe HEAD 2> /dev/null || echo v0)

DATE_FMT = %c
ifdef SOURCE_DATE_EPOCH
	BUILD_INFO ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "+$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "+$(DATE_FMT)" 2>/dev/null || date -u "+$(DATE_FMT)")
else
	BUILD_INFO ?= $(shell date "+$(DATE_FMT)")
endif
NTT_LDFLAGS  = -X 'main.version=$(VERSION)'
NTT_LDFLAGS += -X 'main.commit=$(GIT_COMMIT)'
NTT_LDFLAGS += -X 'main.date=$(BUILD_INFO)'
NTT_LDFLAGS += -X 'main.prefix=$(PREFIX)'

.PHONY: all ## build whole project (default)
all: build

# It's common practice to provide a help target describing available targets.
# This script greps through the Makefile and displays all phony-targets with a
# `##` help string.
.PHONY: help
help:
	@echo Available targets:
	@perl -ne 'printf("\t%-10s\t%s\n", $$1, $$2)  if /^\.PHONY:\s*(.*)\s*##\s*(.*)$$/' <$(MAKEFILE_LIST)

.PHONY: build ## build everything
build: bin/ntt bin/k3

.PHONY: check ## run tests
check:
	$(GO) test ./...

.PHONY: install ## install NTT
install: build
	install -d -m 755 $(DESTDIR)$(BINDIR)
	install -m 755 bin/ntt $(DESTDIR)$(BINDIR)/ntt
	install -m 755 bin/k3 $(DESTDIR)$(BINDIR)/k3
	install -d -m 755 $(DESTDIR)$(DATADIR)/cmake
	install -m 644 cmake/FindNTT.cmake $(DESTDIR)$(DATADIR)/cmake


.PHONY: clean ## delete build artifacts
clean:
	rm -f bin/ntt bin/k3
	rmdir bin

.PHONY: bin/ntt ## build ntt CLI
bin/ntt:
	$(GO_BUILD) -ldflags="$(NTT_LDFLAGS)" -o $@ .

.PHONY: bin/k3 ## build k3 CLI (VM-based run)
bin/k3:
	$(GO_BUILD) -ldflags="$(NTT_LDFLAGS)" -o $@ ./k3

.PHONY: generate ## run code generators
generate:
	$(GO) generate -x ./...

.PHONY: vendor ## vendor dependencies (works only with module support)
vendor:
	GO111MODULE=on $(GO) mod tidy
	GO111MODULE=on $(GO) mod vendor


