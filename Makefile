.PHONY: clean run build install dep test lint format docker migration tools tools-golangci-lint tools-model-garage codegen

# Set the bin path
SHELL := /bin/sh
PATHINSTBIN = $(abspath ./bin)
export PATH := $(PATHINSTBIN):$(PATH)

BIN_NAME					?= server-garage
DEFAULT_INSTALL_DIR			:= $(go env GOPATH)/bin
DEFAULT_ARCH				:= $(shell go env GOARCH)
DEFAULT_GOOS				:= $(shell go env GOOS)
ARCH						?= $(DEFAULT_ARCH)
GOOS						?= $(DEFAULT_GOOS)
INSTALL_DIR					?= $(DEFAULT_INSTALL_DIR)
.DEFAULT_GOAL 				:= run


VERSION   := $(shell git describe --tags || echo "v0.0.0")
VER_CUT   := $(shell echo $(VERSION) | cut -c2-)

# Dependency versions
GOLANGCI_VERSION   = latest
CLICKHOUSE_INFRA_VERSION = $(shell go list -m -f '{{.Version}}' github.com/DIMO-Network/clickhouse-infra)

help:
	@echo "\nSpecify a subcommand:\n"
	@grep -hE '^[0-9a-zA-Z_-]+:.*?## .*$$' ${MAKEFILE_LIST} | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[0;36m%-20s\033[m %s\n", $$1, $$2}'
	@echo ""


all: clean target

clean: ## Clean the project binaries
	@rm -rf $(PATHINSTBIN)
	
tidy:  ## tidy the go modules
	@go mod tidy

test: ## Run the all tests
	@go test ./...

lint: ## Run the linter
	@PATH=$$PATH golangci-lint version
	@PATH=$$PATH golangci-lint run --timeout=5m


generate: generate-go ## Generate all code
	
generate-go: ## Generate the go code for the project
	@go generate ./...

tools: tools-golangci-lint ## Install all tools

tools-golangci-lint: ## Install golangci-lint
	@mkdir -p $(PATHINSTBIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | BINARY=golangci-lint bash -s -- ${GOLANGCI_VERSION}
