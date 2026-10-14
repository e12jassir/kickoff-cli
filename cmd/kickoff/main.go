package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff/internal/catalog"
	"github.com/e12jassir/kickoff/internal/espn"
	"github.com/e12jassir/kickoff/internal/player"
	"github.com/e12jassir/kickoff/internal/ui"
)

const version = "0.1.0"

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("kickoff v%s (Bubble Tea edition)\n", version)
		os.Exit(0)
	}

	client := espn.NewClient()
	cat := catalog.NewResolver()
	play := player.NewController()

	initialModel := ui.NewModel(client, cat, play)
	program := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running kickoff: %v\n", err)
		os.Exit(1)
	}
}
