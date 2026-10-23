package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/bar"
	"github.com/e12jassir/kickoff-cli/internal/catalog"
	"github.com/e12jassir/kickoff-cli/internal/espn"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// ErrNoLiveMatches indicates no matches are currently live (used with -has-live flag).
var ErrNoLiveMatches = errors.New("no live matches")

// RunBar handles the "kickoff bar" subcommand.
func RunBar(args []string, client model.Client, resolver model.Resolver, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	flags := flag.NewFlagSet("bar", flag.ContinueOnError)
	flags.SetOutput(out)

	var format string
	flags.StringVar(&format, "format", "plain", "Output format: plain, starship, tmux, waybar, json")
	flags.StringVar(&format, "f", "plain", "Output format (shorthand)")

	var watch bool
	flags.BoolVar(&watch, "watch", false, "Run continuously in watch mode")
	flags.BoolVar(&watch, "w", false, "Run continuously in watch mode (shorthand)")

	var interval time.Duration
	flags.DurationVar(&interval, "interval", 30*time.Second, "Refresh interval in watch mode")
	flags.DurationVar(&interval, "i", 30*time.Second, "Refresh interval (shorthand)")

	var league string
	flags.StringVar(&league, "league", "", "Filter matches by league key or code (e.g. esp.1, laliga)")
	flags.StringVar(&league, "l", "", "Filter matches by league key or code (shorthand)")

	var team string
	flags.StringVar(&team, "team", "", "Filter matches by team name (case-insensitive substring)")
	flags.StringVar(&team, "t", "", "Filter matches by team name (shorthand)")

	var notify bool
	flags.BoolVar(&notify, "notify", false, "Enable desktop notifications on goals (watch mode)")
	flags.BoolVar(&notify, "n", false, "Enable desktop notifications on goals (shorthand)")

	var hasLive bool
	flags.BoolVar(&hasLive, "has-live", false, "Exit code helper (0 if live matches exist, 1 otherwise)")

	var noEmoji bool
	flags.BoolVar(&noEmoji, "no-emoji", false, "Disable emojis, use ASCII equivalents")

	var demo bool
	flags.BoolVar(&demo, "demo", false, "Use demo client with simulated live matches")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if client == nil {
		if demo {
			client = espn.NewDemoClient()
		} else {
			client = espn.NewClient()
		}
	}
	if resolver == nil {
		resolver = catalog.NewResolver()
	}

	opts := bar.Options{
		Format:  bar.Format(strings.ToLower(format)),
		League:  league,
		Team:    team,
		NoEmoji: noEmoji,
	}

	ctx := context.Background()

	if hasLive {
		matches, err := bar.FetchAndFilter(ctx, client, opts)
		if err != nil {
			return err
		}
		for _, m := range matches {
			if bar.IsLive(m) {
				return nil
			}
		}
		return ErrNoLiveMatches
	}

	if watch {
		sigCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()

		watchOpts := bar.WatchOptions{
			Options:  opts,
			Interval: interval,
			Notify:   notify,
		}
		return bar.Watch(sigCtx, client, resolver, watchOpts, out)
	}

	matches, err := bar.FetchAndFilter(ctx, client, opts)
	if err != nil {
		return err
	}

	output, err := bar.FormatMatches(matches, opts, resolver)
	if err != nil {
		return err
	}

	if output != "" {
		if _, err := fmt.Fprintln(out, output); err != nil {
			return fmt.Errorf("could not write status bar output: %w", err)
		}
	}
	return nil
}
