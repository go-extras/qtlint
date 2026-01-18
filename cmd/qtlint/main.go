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
package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/go-extras/qtlint"
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

	singlechecker.Main(qtlint.Analyzer)
}
