PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: install uninstall

install:
	@mkdir -p $(BINDIR)
	@install -m 755 bin/futbol $(BINDIR)/futbol
	@install -m 755 bin/futbol-preview $(BINDIR)/futbol-preview
	@echo "futbol-cli instalado exitosamente en $(BINDIR)"

uninstall:
	@rm -f $(BINDIR)/futbol $(BINDIR)/futbol-preview
	@echo "futbol-cli desinstalado de $(BINDIR)"
