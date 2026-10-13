PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: all build test run clean install uninstall

all: build

build:
	@mkdir -p bin
	go build -o bin/kickoff ./cmd/kickoff

test:
	go test -v ./...

run:
	go run ./cmd/kickoff

clean:
	rm -rf bin/kickoff

install: build
	@mkdir -p $(BINDIR)
	@install -m 755 bin/kickoff $(BINDIR)/kickoff
	@rm -f $(BINDIR)/kickoff-preview $(BINDIR)/futbol $(BINDIR)/futbol-preview
	@echo "kickoff instalado exitosamente en $(BINDIR)"

uninstall:
	@rm -f $(BINDIR)/kickoff $(BINDIR)/kickoff-preview $(BINDIR)/futbol $(BINDIR)/futbol-preview
	@echo "kickoff desinstalado de $(BINDIR)"
