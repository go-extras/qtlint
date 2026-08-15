// Package modules lets one qtlint invocation cover packages that live in
// several Go modules.
//
// go/packages resolves package patterns by running the go command, and the go
// command resolves them against the module that contains the working
// directory. A pattern naming a different module is not a pattern that matches
// nothing, it is an error:
//
//	$ qtlint ./testkit/...
//	pattern ./testkit/...: directory prefix testkit does not contain main
//	module or its selected dependencies
//
// No spelling avoids this. An absolute path reports the same error, and naming
// the directory without "..." reports that the main module does not contain
// that package. A Go workspace does not fix it either: with a go.work listing
// every module, "./testkit/..." starts working, but "./..." still matches only
// the module the working directory is in, so the caller is still the one
// enumerating modules.
//
// What does work is the working directory. Run the command inside a module and
// it analyzes that module correctly. So this package leaves the driver exactly
// as it is and moves the working directory instead: it finds the modules under
// the requested patterns, then runs the command once per module, each time with
// the working directory set to that module and the patterns rewritten to be
// relative to it.
//
// Re-running the command rather than calling the analysis driver in a loop is
// what keeps the driver whole. golang.org/x/tools/go/analysis/singlechecker
// parses the global flag set and exits the process; it cannot be entered twice
// and it never returns. Replacing it would cost -fix, -diff, -json, -c, the
// -flags inventory that "go vet -vettool" reads, and the .cfg dispatch that
// makes qtlint usable as a vet tool. Each of those keeps working here because
// each child is the same command with the same flags.
package modules

import (
	"path/filepath"
	"slices"
	"strings"
)

// FlagName is the command-line flag that turns multi-module mode on.
//
// The analysis driver already declares -tags, -source, -v, -all, -c, -test,
// -json, -diff, -fix, -flags and the profiling flags, and the flag package
// panics with "flag redefined" on a second registration, so a new flag has to
// keep clear of every name the driver takes. It also keeps clear of -modules,
// which reads like a list of module names to analyze rather than a mode, and
// which golangci-lint has already spent on the unrelated
// --modules-download-mode.
const FlagName = "multi-module"

// childEnv marks a process this package started.
//
// Stripping FlagName from the child's arguments is what stops a child from
// expanding again, and for a boolean flag that stripping is exact: the flag
// package accepts only "-modules", "--modules" and the "=" spellings, all of
// which Strip removes. The marker is here because the failure it guards
// against is a fork bomb rather than a wrong answer, and that is worth a second
// lock. It is not a user-facing setting.
const childEnv = "QTLINT_MODULES_CHILD"

// valueFlags are the flags that take their value as a separate argument.
//
// Splitting flags from package operands needs this and nothing else: a boolean
// flag never consumes the argument after it, so only these can hide an operand
// behind them. "qtlint -c 0 ./..." is the shape that matters — a scan that did
// not know -c takes a value would read "0" as a package pattern.
//
// The driver builds its flag set inside singlechecker.Main, which is after the
// point where this decision has to be made, so the set cannot be read from the
// driver at run time. TestValueFlagsMatchesTheDriver reads the built binary's
// own -h output and fails if this table stops matching it, which turns a future
// x/tools release adding a value-taking flag into a red test rather than a
// misread command line.
var valueFlags = map[string]bool{
	"c":          true,
	"cpuprofile": true,
	"debug":      true,
	"memprofile": true,
	"tags":       true,
	"trace":      true,
}

// ValueFlags returns the driver flags that take their value as a separate
// argument, sorted.
//
// It is exported so that TestValueFlagsMatchesTheDriver, which is next to the
// command because that is where the binary is built, can compare this list
// against the flag set the built driver actually prints. Nothing outside the
// module can see it: this package is internal.
func ValueFlags() []string {
	names := make([]string, 0, len(valueFlags))
	for name := range valueFlags {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// Requested reports whether args ask for multi-module mode, and returns args
// with the flag removed so a child does not expand again.
//
// The value spellings of a boolean flag are honored, so "-modules=false" turns
// the mode off exactly as the flag package would. A repeated flag takes the
// last value, which is also what the flag package does.
func Requested(args []string) (rest []string, on bool) {
	rest = make([]string, 0, len(args))

	for i, arg := range args {
		// The flag package stops parsing flags at a bare "--", so a
		// "-modules" after it is a package pattern rather than a flag.
		if arg == "--" {
			rest = append(rest, args[i:]...)

			break
		}

		name, value, isFlag := splitFlag(arg)
		if !isFlag || name != FlagName {
			rest = append(rest, arg)

			continue
		}

		// A boolean flag never takes the following argument as its value,
		// so anything not written with "=" is simply true.
		on = value != "false"
	}

	return rest, on
}

// SplitArgs splits args into the leading flags and the package operands that
// follow them, the way the flag package does.
//
// Parsing stops at the first argument that is not a flag and is not the value
// of one, and everything from there on is an operand. A bare "--" also ends the
// flags, and is kept with them so that reassembling a command line preserves
// it.
func SplitArgs(args []string) (flags, operands []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			return args[:i+1], args[i+1:]
		}

		name, _, isFlag := splitFlag(args[i])
		if !isFlag {
			return args[:i], args[i:]
		}

		// A value written with "=" is part of this argument; otherwise a
		// value-taking flag swallows the next one.
		if !strings.Contains(args[i], "=") && valueFlags[name] && i+1 < len(args) {
			i++
		}
	}

	return args, nil
}

// splitFlag reports whether arg is shaped like a flag and, if so, returns its
// name and any value joined to it with "=".
//
// The flag package accepts one leading dash or two, and treats "-", "--" and
// anything not starting with a dash as something other than a flag.
func splitFlag(arg string) (name, value string, ok bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if trimmed == "" || strings.HasPrefix(trimmed, "-") {
		return "", "", false
	}

	name, value, _ = strings.Cut(trimmed, "=")

	return name, value, true
}

// isDirPattern reports whether pattern names a directory rather than an import
// path or one of the go command's reserved pattern words.
//
// Multi-module mode has to turn a pattern into a set of directories to look for
// go.mod files in, and only a directory pattern carries that information. An
// import path such as "example.com/x/..." names packages the go command
// resolves through the module graph, and "all", "std", "cmd" and "tool" name
// sets that have no directory root at all. Refusing those is deliberate: the
// alternative is to analyze whatever subset happens to resolve and exit 0,
// which is the shape of a run that looks clean because it inspected nothing.
func isDirPattern(pattern string) bool {
	if filepath.IsAbs(pattern) {
		return true
	}

	cleaned := filepath.ToSlash(pattern)

	return cleaned == "." || cleaned == ".." ||
		strings.HasPrefix(cleaned, "./") || strings.HasPrefix(cleaned, "../")
}
