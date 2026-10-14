package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff/internal/catalog"
	"github.com/e12jassir/kickoff/internal/espn"
	"github.com/e12jassir/kickoff/internal/player"
	"github.com/e12jassir/kickoff/internal/ui"
	"github.com/e12jassir/kickoff/internal/updater"
)

const version = "0.1.1"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
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
		case "version":
			fmt.Printf("kickoff v%s (Bubble Tea edition)\n", version)
			os.Exit(0)
		case "help", "-h", "--help":
			fmt.Printf("kickoff v%s — Terminal live football streaming & score tracker\n\n", version)
			fmt.Println("Comandos:")
			fmt.Println("  kickoff          Abre el explorador interactivo de partidos en la terminal")
			fmt.Println("  kickoff update   Actualiza kickoff automáticamente a la última versión")
			fmt.Println("  kickoff setup    Diagnostica y verifica dependencias del sistema (mpv, fuentes)")
			fmt.Println("  kickoff version  Muestra la versión actual")
			os.Exit(0)
		}
	}

	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("kickoff v%s (Bubble Tea edition)\n", version)
		os.Exit(0)
	}

	client := espn.NewClient()
	cat := catalog.NewResolver()
	play := player.NewController()

	initialModel := ui.NewModel(client, cat, play).WithVersion(version)
	program := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running kickoff: %v\n", err)
		os.Exit(1)
	}
}
