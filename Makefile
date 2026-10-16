VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "unknown")
BINARY_NAME=bin/kickoff
SRC=./cmd/kickoff
INSTALL_DIR=$(HOME)/.local/bin
CONFIG_DIR=$(HOME)/.config/kickoff

.PHONY: all build install test coverage vet lint clean setup-channels

all: build

build:
	@mkdir -p bin
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" -o $(BINARY_NAME) $(SRC)

setup-channels:
	@mkdir -p $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/channels.json ]; then \
		echo "Inicializando configuración de canales en $(CONFIG_DIR)/channels.json..."; \
		cp -n channels.json.example $(CONFIG_DIR)/channels.json 2>/dev/null || true; \
	fi

install: build setup-channels
	@mkdir -p $(INSTALL_DIR)
	@install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/kickoff
	@chmod +x $(INSTALL_DIR)/kickoff
	@echo "kickoff instalado exitosamente en $(INSTALL_DIR)"

test:
	go test -race -count=1 ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

vet:
	go vet ./...

# Requires golangci-lint to be installed (https://golangci-lint.run).
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed. See https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	}
	golangci-lint run

clean:
	@rm -rf bin
