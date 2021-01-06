.PHONY: prepare all init help dist fmt clean run install release

GOCMD       = go
GOBUILD     = $(GOCMD) build
GOCLEAN     = $(GOCMD) clean
GOTEST      = $(GOCMD) test
GOGET       = $(GOCMD) get
GORUN       = $(GOCMD) run

BUILDDIR    = ./build
BINDIR      = $(BUILDDIR)/bin
PKGDIR      = $(BUILDDIR)/pkg
DISTDIR     = $(BUILDDIR)/dist


NAME        := mockapi
VERSION     := 0.9.5
CURRENT     := $(shell pwd)
ENTRY       := $(CURRENT)/main.go
SRCS        := $(shell find . -type f -name '*.go')
GOXOS       := linux
GOXARCH     := amd64
GOXOUTPUT   := $(PKGDIR)/$(GOXOS)_$(GOXARCH)/$(NAME)

INSTALLHOST := me.nginx
INSTALLPATH := /data/okcoin/okj-agent


all: help

help:
	@echo "Build and distribute okj-agent app"
	@echo "    prepare                    check golang env"
	@echo "    init                       init mod"
	@echo "    run                        run main file"
	@echo "    build                      compile on local platform, MacOS etc."
	@echo "    dist                       compile and generate $(GOXOS)_$(GOXARCH) binary"
	@echo "    install                    upload $(GOXOUTPUT) to $(INSTALLHOST):$(INSTALLPATH)"
	@echo "    fmt                        format source code"
	@echo "    release                    release a version, tag to git repository"
	@echo "    clean                      clean build output"

prepare:
	@echo check golang env
	$(GOCMD) env

init:
	@echo init mod
	@echo $(VERSION) $(REVISION)
	[ -f $(CURRENT)/go.mod ] || $(GOCMD) mod init

run: init
	@echo run main file : $(ENTRY)
	$(GORUN) $(ENTRY)

build: init
	@echo $(SRCS) $(BINDIR)/$(NAME)
	@echo $(subst .go,,$(SRCS))
	rm -rf $(BINDIR)
	$(GOBUILD) -o $(BINDIR)/$(NAME) $(SRCS)

dist: init
	@echo build $(GOXOS)_$(GOXARCH) binary
	rm -rf $(PKGDIR)
	CGO_ENABLED=0 GOOS=$(GOXOS) GOARCH=$(GOXARCH) $(GOBUILD) -o $(GOXOUTPUT) $(SRCS)

install: dist
	@echo upload $(GOXOUTPUT) to $(INSTALLHOST)
	scp $(GOXOUTPUT) $(INSTALLHOST):$(INSTALLPATH)

release: build
	git tag $(VERSION)
	git push origin $(VERSION)

fmt: prepare
	find . -name "*.go" -not -path "./vendor/*" | xargs goimports -w

clean:
	$(GOCLEAN)
	rm -rf $(BUILDDIR)
