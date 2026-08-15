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
	"github.com/go-extras/qtlint/internal/tagsflag"
)

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
