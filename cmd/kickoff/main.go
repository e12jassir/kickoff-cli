package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff-cli/internal/catalog"
	"github.com/e12jassir/kickoff-cli/internal/espn"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/player"
	"github.com/e12jassir/kickoff-cli/internal/ui"
	"github.com/e12jassir/kickoff-cli/internal/updater"
	"github.com/muesli/termenv"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func printVersion() {
	fmt.Printf("kickoff version: %s (commit: %s, built at: %s)\n", version, commit, date)
}

func printHelp() {
	vStr := version
	if !strings.HasPrefix(vStr, "v") && vStr != "dev" {
		vStr = "v" + vStr
	}
	fmt.Printf("kickoff %s — Terminal live football streaming & score tracker\n\n", vStr)
	fmt.Println("Commands:")
	fmt.Println("  kickoff                     Open interactive match explorer in terminal")
	fmt.Println("  kickoff demo                Open terminal with simulated live matches (demo mode)")
	fmt.Println("  kickoff init                Initialize channels.json with an example starter template")
	fmt.Println("  kickoff config import <src> Import channels from local file or URL (JSON or M3U playlist)")
	fmt.Println("  kickoff import <src>        Shortcut for 'kickoff config import'")
	fmt.Println("  kickoff update              Update kickoff automatically to latest release")
	fmt.Println("  kickoff setup               Diagnose and verify system dependencies (mpv, fonts)")
	fmt.Println("  kickoff version             Print version information and exit")
	fmt.Println("\nFlags:")
	fmt.Println("  -demo                       Launch TUI in demo mode with simulated matches")
	fmt.Println("  -no-emoji                   Disable emoji rendering and use clean ASCII alternatives")
	fmt.Println("  -version, --version, -v     Print version information and exit")
}

func main() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}

	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	runDemo := false
	noEmoji := false

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initFlags := flag.NewFlagSet("init", flag.ExitOnError)
			force := initFlags.Bool("force", false, "Overwrite existing configuration file")
			initFlags.BoolVar(force, "f", false, "Overwrite existing configuration file (shorthand)")
			_ = initFlags.Parse(os.Args[2:])
			if err := RunInit(*force); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "config":
			if len(os.Args) > 2 && os.Args[2] == "import" {
				if len(os.Args) < 4 {
					fmt.Fprintln(os.Stderr, "Usage: kickoff config import <url|path>")
					os.Exit(1)
				}
				if err := RunConfigImport(os.Args[3]); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "Unknown command. Use 'kickoff config import <url|path>'")
			os.Exit(1)
		case "import":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: kickoff import <url|path>")
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
		case "version", "-v", "-version", "--version":
			printVersion()
			os.Exit(0)
		case "help", "-h", "--help":
			printHelp()
			os.Exit(0)
		}
	}

	demoFlag := flag.Bool("demo", false, "Open TUI in demo mode with simulated live matches")
	noEmojiFlag := flag.Bool("no-emoji", false, "Disable emoji rendering and use clean ASCII alternatives")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	shortVersionFlag := flag.Bool("v", false, "Print version information and exit (shorthand)")
	flag.Parse()

	if *versionFlag || *shortVersionFlag {
		printVersion()
		os.Exit(0)
	}

	if *noEmojiFlag {
		noEmoji = true
	}

	for _, arg := range os.Args[1:] {
		if arg == "--no-emoji" || arg == "-no-emoji" {
			noEmoji = true
		}
	}

	var client model.Client = espn.NewClient()
	if *demoFlag || runDemo {
		client = espn.NewDemoClient()
	}
	cat := catalog.NewResolver()
	play := player.NewController()

	initialModel := ui.NewModel(client, cat, play).WithVersion(version).WithNoEmoji(noEmoji).WithInitialState(ui.StateStartup)
	program := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running kickoff: %v\n", err)
		os.Exit(1)
	}
}
