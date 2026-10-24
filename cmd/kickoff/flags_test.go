package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/i18n"
)

func TestPrintHelp_Localization(t *testing.T) {
	// Spanish
	var bufES bytes.Buffer
	printHelp(&bufES, "v0.3.0", i18n.New(i18n.ES))
	outES := bufES.String()

	if !strings.Contains(outES, "kickoff v0.3.0 — Transmisión de fútbol en vivo") {
		t.Errorf("expected Spanish header in help, got:\n%s", outES)
	}
	if !strings.Contains(outES, "Comandos:") {
		t.Errorf("expected 'Comandos:' in Spanish help")
	}
	if !strings.Contains(outES, "Banderas:") {
		t.Errorf("expected 'Banderas:' in Spanish help")
	}
	if !strings.Contains(outES, "Abrir explorador interactivo de partidos en la terminal") {
		t.Errorf("expected localized command description in Spanish help")
	}

	// English
	var bufEN bytes.Buffer
	printHelp(&bufEN, "v0.3.0", i18n.New(i18n.EN))
	outEN := bufEN.String()

	if !strings.Contains(outEN, "kickoff v0.3.0 — Terminal live football streaming") {
		t.Errorf("expected English header in help, got:\n%s", outEN)
	}
	if !strings.Contains(outEN, "Commands:") {
		t.Errorf("expected 'Commands:' in English help")
	}
	if !strings.Contains(outEN, "Flags:") {
		t.Errorf("expected 'Flags:' in English help")
	}
	if !strings.Contains(outEN, "Open interactive match explorer in terminal") {
		t.Errorf("expected localized command description in English help")
	}
}

func TestRunBar_Help_Localization(t *testing.T) {
	// Spanish help
	var bufES bytes.Buffer
	errES := RunBar([]string{"-h"}, nil, nil, &bufES, i18n.New(i18n.ES))
	if errES != nil {
		t.Fatalf("unexpected error on -h: %v", errES)
	}
	outES := bufES.String()
	if !strings.Contains(outES, "Formato de salida") {
		t.Errorf("expected Spanish flag help, got:\n%s", outES)
	}
	if !strings.Contains(outES, "Intervalo de actualización en modo monitor") {
		t.Errorf("expected Spanish interval description, got:\n%s", outES)
	}

	// English help
	var bufEN bytes.Buffer
	errEN := RunBar([]string{"-h"}, nil, nil, &bufEN, i18n.New(i18n.EN))
	if errEN != nil {
		t.Fatalf("unexpected error on -h: %v", errEN)
	}
	outEN := bufEN.String()
	if !strings.Contains(outEN, "Output format") {
		t.Errorf("expected English flag help, got:\n%s", outEN)
	}
	if !strings.Contains(outEN, "Refresh interval in watch mode") {
		t.Errorf("expected English interval description, got:\n%s", outEN)
	}
}

func TestBuildMainFlagSet_Localization(t *testing.T) {
	catES := i18n.New(i18n.ES)
	fsES, mfES := buildMainFlagSet(catES)
	if mfES == nil || fsES == nil {
		t.Fatal("expected non-nil flag set and holder")
	}

	demoFlagES := fsES.Lookup("demo")
	if demoFlagES == nil || demoFlagES.Usage != catES.CLIFlagDemo {
		t.Errorf("expected localized usage %q, got %v", catES.CLIFlagDemo, demoFlagES)
	}

	catEN := i18n.New(i18n.EN)
	fsEN, _ := buildMainFlagSet(catEN)
	demoFlagEN := fsEN.Lookup("demo")
	if demoFlagEN == nil || demoFlagEN.Usage != catEN.CLIFlagDemo {
		t.Errorf("expected localized usage %q, got %v", catEN.CLIFlagDemo, demoFlagEN)
	}
}

func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestRunInit_Localization(t *testing.T) {
	_, cfgDir := isolateConfigHome(t)

	// First initialization with Spanish catalog
	outES := captureStdout(func() {
		_ = RunInit(false, i18n.New(i18n.ES))
	})
	if !strings.Contains(outES, "Archivo channels.json inicializado con éxito") {
		t.Errorf("expected Spanish init output, got: %s", outES)
	}

	// Second run without force with Spanish catalog
	outExistsES := captureStdout(func() {
		_ = RunInit(false, i18n.New(i18n.ES))
	})
	if !strings.Contains(outExistsES, "El archivo de configuración ya existe en:") {
		t.Errorf("expected Spanish exists output, got: %s", outExistsES)
	}

	// Overwrite with English catalog
	outOverwrittenEN := captureStdout(func() {
		_ = RunInit(true, i18n.New(i18n.EN))
	})
	if !strings.Contains(outOverwrittenEN, "File channels.json successfully overwritten at:") {
		t.Errorf("expected English overwritten output, got: %s", outOverwrittenEN)
	}
	_ = cfgDir
}

func TestRunConfigImport_Localization(t *testing.T) {
	_, _ = isolateConfigHome(t)
	tmpFile := filepath.Join(t.TempDir(), "test_channels.json")
	if err := os.WriteFile(tmpFile, []byte(`{"streams":{"espn":"http://example.com/live.m3u8"}}`), 0o644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	// Import with Spanish catalog
	outES := captureStdout(func() {
		_ = RunConfigImport(tmpFile, i18n.New(i18n.ES))
	})
	if !strings.Contains(outES, "¡Configuración importada con éxito con 1 señales configuradas!") {
		t.Errorf("expected Spanish import output, got: %s", outES)
	}
	if !strings.Contains(outES, "Configuración guardada en:") {
		t.Errorf("expected Spanish save path output, got: %s", outES)
	}

	// Import with English catalog
	outEN := captureStdout(func() {
		_ = RunConfigImport(tmpFile, i18n.New(i18n.EN))
	})
	if !strings.Contains(outEN, "Successfully imported configuration with 1 configured stream endpoints!") {
		t.Errorf("expected English import output, got: %s", outEN)
	}
	if !strings.Contains(outEN, "Configuration saved to:") {
		t.Errorf("expected English save path output, got: %s", outEN)
	}
}
