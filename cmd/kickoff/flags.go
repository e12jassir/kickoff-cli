package main

import (
	"flag"
	"io"
	"strings"

	"github.com/e12jassir/kickoff-cli/internal/i18n"
)

type mainFlagsHolder struct {
	demo         *bool
	noEmoji      *bool
	version      *bool
	versionShort *bool
}

func printHelp(out io.Writer, ver string, optCat ...*i18n.Catalog) {
	cat := i18n.Default()
	if len(optCat) > 0 && optCat[0] != nil {
		cat = optCat[0]
	}

	vStr := ver
	if !strings.HasPrefix(vStr, "v") && vStr != "dev" {
		vStr = "v" + vStr
	}

	var buf strings.Builder
	buf.WriteString(cat.CLIHelpHeader(vStr) + "\n")
	buf.WriteString(cat.CLIHelpCommandsHeader + "\n")
	buf.WriteString(cat.CLIHelpCommandKickoff + "\n")
	buf.WriteString(cat.CLIHelpCommandDemo + "\n")
	buf.WriteString(cat.CLIHelpCommandBar + "\n")
	buf.WriteString(cat.CLIHelpCommandInit + "\n")
	buf.WriteString(cat.CLIHelpCommandConfigImport + "\n")
	buf.WriteString(cat.CLIHelpCommandImport + "\n")
	buf.WriteString(cat.CLIHelpCommandUpdate + "\n")
	buf.WriteString(cat.CLIHelpCommandSetup + "\n")
	buf.WriteString(cat.CLIHelpCommandVersion + "\n\n")
	buf.WriteString(cat.CLIHelpFlagsHeader + "\n")
	buf.WriteString("  -demo                       " + cat.CLIFlagDemo + "\n")
	buf.WriteString("  -no-emoji                   " + cat.CLIFlagNoEmoji + "\n")
	buf.WriteString("  -version, --version, -v     " + cat.CLIFlagVersion + "\n")

	_, _ = io.WriteString(out, buf.String())
}

func buildMainFlagSet(cat *i18n.Catalog) (*flag.FlagSet, *mainFlagsHolder) {
	if cat == nil {
		cat = i18n.Default()
	}

	fs := flag.NewFlagSet("kickoff", flag.ContinueOnError)
	holder := &mainFlagsHolder{
		demo:         fs.Bool("demo", false, cat.CLIFlagDemo),
		noEmoji:      fs.Bool("no-emoji", false, cat.CLIFlagNoEmoji),
		version:      fs.Bool("version", false, cat.CLIFlagVersion),
		versionShort: fs.Bool("v", false, cat.CLIFlagVersionShort),
	}
	return fs, holder
}
