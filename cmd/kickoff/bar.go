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
	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// ErrNoLiveMatches indicates no matches are currently live (used with -has-live flag).
var ErrNoLiveMatches = errors.New("no live matches")

// RunBar handles the "kickoff bar" subcommand.
func RunBar(args []string, client model.Client, resolver model.Resolver, out io.Writer, optCat ...*i18n.Catalog) error {
	if out == nil {
		out = os.Stdout
	}

	cat := i18n.Default()
	if len(optCat) > 0 && optCat[0] != nil {
		cat = optCat[0]
	}

	flags := flag.NewFlagSet("bar", flag.ContinueOnError)
	flags.SetOutput(out)

	var format string
	flags.StringVar(&format, "format", "plain", cat.CLIBarFlagFormat)
	flags.StringVar(&format, "f", "plain", cat.CLIBarFlagFormatShort)

	var watch bool
	flags.BoolVar(&watch, "watch", false, cat.CLIBarFlagWatch)
	flags.BoolVar(&watch, "w", false, cat.CLIBarFlagWatchShort)

	var interval time.Duration
	flags.DurationVar(&interval, "interval", 30*time.Second, cat.CLIBarFlagInterval)
	flags.DurationVar(&interval, "i", 30*time.Second, cat.CLIBarFlagIntervalShort)

	var league string
	flags.StringVar(&league, "league", "", cat.CLIBarFlagLeague)
	flags.StringVar(&league, "l", "", cat.CLIBarFlagLeagueShort)

	var team string
	flags.StringVar(&team, "team", "", cat.CLIBarFlagTeam)
	flags.StringVar(&team, "t", "", cat.CLIBarFlagTeamShort)

	var notify bool
	flags.BoolVar(&notify, "notify", false, cat.CLIBarFlagNotify)
	flags.BoolVar(&notify, "n", false, cat.CLIBarFlagNotifyShort)

	var hasLive bool
	flags.BoolVar(&hasLive, "has-live", false, cat.CLIBarFlagHasLive)

	var noEmoji bool
	flags.BoolVar(&noEmoji, "no-emoji", false, cat.CLIBarFlagNoEmoji)

	var demo bool
	flags.BoolVar(&demo, "demo", false, cat.CLIBarFlagDemo)

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

	output, err := bar.FormatMatches(matches, opts, resolver, cat)
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
