// Command qtlint is a standalone runner for the qtlint analyzer.
//
// It can be used to run the linter independently without golangci-lint.
//
// Usage:
//
//	qtlint [flags] [packages]
//
// Examples:
//
//	# Analyze current package
//	qtlint .
//
//	# Analyze specific packages
//	qtlint ./...
//
//	# Analyze packages and files behind a build constraint
//	qtlint -tags integration ./...
//	qtlint -tags integration,e2e ./...
package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/go-extras/qtlint"
	"github.com/go-extras/qtlint/internal/modules"
	"github.com/go-extras/qtlint/internal/tagsflag"
)

// executable returns the path to re-run for each module.
//
// The reliable spelling is os.Executable. Argument zero is whatever the caller
// typed, and it need not resolve from another working directory, which is
// exactly where each module run puts it. Falling back to it is still better
// than refusing, because a relative argument zero only fails once the run
// starts, and says so plainly when it does.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}

	return exe
}

// workingDir returns the directory package patterns are written relative to.
func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}

	return wd
}

// Build information. Populated at build-time via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Check for version flag before singlechecker processes flags.
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" || arg == "-V" {
			fmt.Printf("qtlint version %s (commit: %s, built: %s)\n", version, commit, date)
			os.Exit(0)
		}
	}

	// Add custom version flag.
	flag.Bool("version", false, "print version and exit")

	// Registered so that -h describes it and the driver's own parse accepts
	// it; the mode itself is decided below, before the driver ever parses.
	flag.Bool(modules.FlagName, false, modules.Usage)

	// A pattern naming a module other than the one holding the working
	// directory is an error the go command reports, whatever the spelling, so
	// covering several modules means running once per module with the working
	// directory moved. That has to happen before the driver starts, because
	// the driver parses the global flag set and exits without returning. See
	// package modules.
	if code, handled, err := modules.Dispatch(
		executable(), workingDir(), os.Args[1:], os.Stdout, os.Stderr,
	); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "qtlint: %v\n", err)
			os.Exit(1)
		}

		os.Exit(code)
	}

	// The analysis driver accepts -tags but does nothing with it, and it never
	// hands out the package-loading configuration where build tags would
	// belong. Put them where the go command that go/packages runs will read
	// them, before the driver starts loading. See package tagsflag.
	if goflags, ok := tagsflag.Forward(os.Args[1:], os.Getenv("GOFLAGS")); ok {
		if err := os.Setenv("GOFLAGS", goflags); err != nil {
			fmt.Fprintf(os.Stderr, "qtlint: cannot forward -tags through GOFLAGS: %v\n", err)
			os.Exit(1)
		}
	}

	// The driver documents its own -tags shim as having no effect. It does
	// have one here, so correct that line on its way out.
	flag.CommandLine.SetOutput(tagsflag.UsageWriter(os.Stderr))

	singlechecker.Main(qtlint.Analyzer)
}
