VERSION ?= 0.1.2
BINARY_NAME=bin/kickoff
SRC=./cmd/kickoff
INSTALL_DIR=$(HOME)/.local/bin
CONFIG_DIR=$(HOME)/.config/kickoff

.PHONY: all build install test clean setup-channels

all: build

build:
	@mkdir -p bin
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY_NAME) $(SRC)

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
	go test -v ./...

clean:
	@rm -rf bin
