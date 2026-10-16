package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/catalog"
	"github.com/e12jassir/kickoff-cli/internal/espn"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/player"
	"github.com/e12jassir/kickoff-cli/internal/ui"
	"github.com/e12jassir/kickoff-cli/internal/updater"
)

var version = "0.1.2"

func main() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}

	runDemo := false
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initFlags := flag.NewFlagSet("init", flag.ExitOnError)
			force := initFlags.Bool("force", false, "Sobrescribir archivo existente")
			initFlags.BoolVar(force, "f", false, "Sobrescribir archivo existente (abreviado)")
			_ = initFlags.Parse(os.Args[2:])
			if err := RunInit(*force); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "config":
			if len(os.Args) > 2 && os.Args[2] == "import" {
				if len(os.Args) < 4 {
					fmt.Fprintln(os.Stderr, "Uso: kickoff config import <url|path>")
					os.Exit(1)
				}
				if err := RunConfigImport(os.Args[3]); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "Comando desconocido. Usa 'kickoff config import <url|path>'")
			os.Exit(1)
		case "import":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Uso: kickoff import <url|path>")
				os.Exit(1)
			}
			if err := RunConfigImport(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "update":
			if err := updater.SelfUpdate(context.Background(), "", version); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "setup":
			if err := updater.RunSetup(); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		case "demo":
			runDemo = true
		case "version":
			fmt.Printf("kickoff v%s (Bubble Tea edition)\n", version)
			os.Exit(0)
		case "help", "-h", "--help":
			fmt.Printf("kickoff v%s — Terminal live football streaming & score tracker\n\n", version)
			fmt.Println("Comandos:")
			fmt.Println("  kickoff                     Abre el explorador interactivo de partidos en la terminal")
			fmt.Println("  kickoff demo                Abre la terminal con partidos simulados en vivo (modo demo)")
			fmt.Println("  kickoff init                Inicializa el archivo channels.json con una plantilla de ejemplo")
			fmt.Println("  kickoff config import <src> Importa canales desde archivo local o URL (JSON o lista M3U)")
			fmt.Println("  kickoff import <src>        Atajo para 'kickoff config import'")
			fmt.Println("  kickoff update              Actualiza kickoff automáticamente a la última versión")
			fmt.Println("  kickoff setup               Diagnostica y verifica dependencias del sistema (mpv, fuentes)")
			fmt.Println("  kickoff version             Muestra la versión actual")
			os.Exit(0)
		}
	}

	demoFlag := flag.Bool("demo", false, "Abre la TUI en modo demo con partidos simulados en vivo")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("kickoff v%s (Bubble Tea edition)\n", version)
		os.Exit(0)
	}

	var client model.Client = espn.NewClient()
	if *demoFlag || runDemo {
		client = espn.NewDemoClient()
	}
	cat := catalog.NewResolver()
	play := player.NewController()

	initialModel := ui.NewModel(client, cat, play).WithVersion(version)
	program := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running kickoff: %v\n", err)
		os.Exit(1)
	}
}
