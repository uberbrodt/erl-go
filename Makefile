# this is a GNU make file!!!! Make sure you run with GNU make
# eg `alias make=gmake` on Mac OS X
.PHONY: clean build test test-integration coverage-report test-watch view-docs format check-tools check ci help

TEST_ARG ?= ./...
TEST_RUN ?= ""
TIMEOUT ?= "3m"
version_file := VERSION
VERSION := $(shell cat ${version_file})

ifdef GOOS
GOOS := $(GOOS)
else
GOOS := $(shell go env GOOS)
endif

ifeq (darwin, $(GOOS))
MAKECMD := gmake
else
MAKECMD := make
endif



#################################################################################
#@ clean: remove build artifacts
#################################################################################
clean:
	go clean ./...
	go clean -cache


#################################################################################
#@ build: build the app, including anything that needs to be generated
#################################################################################
build:
	GOOS=$(GOOS) go build ./...

#################################################################################
#@ release: release the app, including anything that needs to be generated
#################################################################################
release:
	@echo $(VERSION)
	gh release create v$(VERSION) --generate-notes

#################################################################################
#@ pre-release: same as relase, but mark as draft
#################################################################################
pre-release:
	@echo $(VERSION)
	gh release create v$(VERSION) --generate-notes --prerelease

#################################################################################
#@ test: runs all tests.
#################################################################################
test: check-tools
	GORACE="history_size=2" gotestsum -f testname -- -timeout $(TIMEOUT) -race -run=$(TEST_RUN) -coverprofile cover.out $(TEST_ARG)
	./scripts/rm-test-fw-from-coverprofile


#################################################################################
#@ test-full: runs all tests, including slow and integration tests.
#################################################################################
test-full:
	SLOW=1 gotestsum -f testname -- -tags=integration -p 1 -coverprofile cover.out $(TEST_ARG)


#################################################################################
#@ coverage-report: writes coverage report to cover.out and opens the webbrowser to the report.
#################################################################################
coverage-report:
	go tool cover --html=cover.out


#################################################################################
#@ test-watch: run test suite anytime a *.go file is changed
#################################################################################
test-watch:
	reflex -s -r '\.go$$' $(MAKECMD) test

#################################################################################
#@ view-docs: start the documentation server
#################################################################################
view-docs:
ifeq (, $(shell which pkgsite))
	go install golang.org/x/pkgsite/cmd/pkgsite@latest
endif
	@echo "To view docs, go to: http://localhost:8081/github.com/uberbrodt/erl-go"
	@echo "" #new line
	pkgsite -http localhost:8081


#################################################################################
#@ format: checks that project code is formatted
#################################################################################
format: check-tools
	gofumpt -w -l .


#################################################################################
#@ check-tools: installs necessary tools for check, test, etc.
#################################################################################
check-tools:
ifeq (, $(shell which gotestsum))
	go install gotest.tools/gotestsum@latest
endif
ifeq (, $(shell which gofumpt))
	go install mvdan.cc/gofumpt@latest
endif
ifeq (, $(shell which reflex))
	go install github.com/cespare/reflex@latest
endif

#################################################################################
#@ check: run static checks against source, used in CI
#################################################################################
check: check-tools
	./scripts/check-formatting
	./scripts/check-test-build-tags


#################################################################################
#@ help: Prints this help message
#################################################################################
help:
	@echo "Usage: \n"
	@sed -n 's/^#@//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

