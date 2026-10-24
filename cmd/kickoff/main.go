package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff-cli/internal/catalog"
	"github.com/e12jassir/kickoff-cli/internal/espn"
	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/player"
	"github.com/e12jassir/kickoff-cli/internal/setup"
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
	v := version
	if !strings.HasPrefix(v, "v") && v != "dev" {
		v = "v" + v
	}
	fmt.Printf("kickoff %s\n", v)
}

func main() {
	// Build provenance: version, commit and date are injected at link time by
	// the release build (`-ldflags -X main.commit=... -X main.date=...`, see the
	// Makefile build target and .github/workflows/release.yml). Only version is
	// printed today, so commit and date are referenced here to keep those linker
	// targets live while --version and --help keep their documented output.
	_ = commit + date

	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}

	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	cat := i18n.Default()

	if strings.HasSuffix(filepath.Base(os.Args[0]), "kickoff-setup") {
		launch, err := setup.Run(version, os.Args[1:]...)
		if err != nil {
			os.Exit(1)
		}
		if !launch {
			os.Exit(0)
		}
	}

	runDemo := false

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initFlags := flag.NewFlagSet("init", flag.ExitOnError)
			force := initFlags.Bool("force", false, cat.CLIInitForceFlag)
			initFlags.BoolVar(force, "f", false, cat.CLIInitForceFlagShort)
			_ = initFlags.Parse(os.Args[2:])
			if err := RunInit(*force, cat); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "config":
			if len(os.Args) > 2 && os.Args[2] == "import" {
				if len(os.Args) < 4 {
					fmt.Fprintln(os.Stderr, cat.CLIUsageConfigImport)
					os.Exit(1)
				}
				if err := RunConfigImport(os.Args[3], cat); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, cat.CLIUnknownConfigCommand)
			os.Exit(1)
		case "import":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, cat.CLIUsageImport)
				os.Exit(1)
			}
			if err := RunConfigImport(os.Args[2], cat); err != nil {
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
			launch, err := setup.Run(version, os.Args[2:]...)
			if err != nil {
				os.Exit(1)
			}
			if !launch {
				os.Exit(0)
			}
			cat = i18n.Default()
		case "bar":
			if err := RunBar(os.Args[2:], nil, nil, os.Stdout, cat); err != nil {
				if errors.Is(err, ErrNoLiveMatches) {
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "demo":
			runDemo = true
		case "version", "-v", "-version", "--version":
			printVersion()
			os.Exit(0)
		case "help", "-h", "--help":
			printHelp(os.Stdout, version, cat)
			os.Exit(0)
		}
	}

	demoFlag := flag.Bool("demo", false, cat.CLIFlagDemo)
	noEmojiFlag := flag.Bool("no-emoji", false, cat.CLIFlagNoEmoji)
	versionFlag := flag.Bool("version", false, cat.CLIFlagVersion)
	shortVersionFlag := flag.Bool("v", false, cat.CLIFlagVersionShort)
	flag.Usage = func() {
		printHelp(flag.CommandLine.Output(), version, cat)
	}
	flag.Parse()

	if *versionFlag || *shortVersionFlag {
		printVersion()
		os.Exit(0)
	}

	// flag.Parse stops at the first non-flag argument, so a trailing
	// --no-emoji (e.g. `kickoff demo --no-emoji`) never reaches the flag.
	// Honor the explicit token wherever it appears in the args.
	noEmoji := *noEmojiFlag
	for _, arg := range os.Args[1:] {
		if arg == "-no-emoji" || arg == "--no-emoji" {
			noEmoji = true
		}
	}

	var client model.Client = espn.NewClient()
	if *demoFlag || runDemo {
		client = espn.NewDemoClient()
	}
	resolver := catalog.NewResolver()
	play := player.NewController()

	initialModel := ui.NewModel(client, resolver, play).
		WithVersion(version).
		WithNoEmoji(noEmoji).
		WithI18n(cat).
		WithInitialState(ui.StateStartup)
	program := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running kickoff: %v\n", err)
		os.Exit(1)
	}
}
