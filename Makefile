PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: install uninstall

install:
	@mkdir -p $(BINDIR)
	@install -m 755 bin/kickoff $(BINDIR)/kickoff
	@install -m 755 bin/kickoff-preview $(BINDIR)/kickoff-preview
	@rm -f $(BINDIR)/futbol $(BINDIR)/futbol-preview
	@echo "kickoff instalado exitosamente en $(BINDIR)"

uninstall:
	@rm -f $(BINDIR)/kickoff $(BINDIR)/kickoff-preview $(BINDIR)/futbol $(BINDIR)/futbol-preview
	@echo "kickoff desinstalado de $(BINDIR)"
