package modules

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Usage is the description of the flag, shown by -h.
const Usage = "analyze every module found under the given directory patterns, " +
	"running the linter once per module"

// IsChild reports whether this process was started by a multi-module parent.
func IsChild() bool { return os.Getenv(childEnv) != "" }

// Dispatch runs the multi-module expansion when args ask for it, and reports
// whether it handled the invocation.
//
// A false report means this invocation is an ordinary one and the caller should
// go on to the analysis driver unchanged, which is what keeps single-module
// behavior exactly as it was: nothing here runs unless the flag is present.
func Dispatch(exe, wd string, args []string, stdout, stderr io.Writer) (code int, handled bool, err error) {
	rest, on := Requested(args)
	if !on {
		return 0, false, nil
	}

	// A child re-running for one module must analyze that module rather than
	// expand again, and Requested having removed the flag from the arguments
	// it hands the child is what ensures that. Reaching here as a child means
	// that removal stopped working, so it is reported rather than absorbed:
	// the failure it leads to is unbounded process creation, and a guard that
	// silently covers for a broken one leaves nothing to notice.
	if IsChild() {
		return 0, true, fmt.Errorf(
			"-%s reached a child process, which would expand again; "+
				"this is a bug in qtlint, not in the command line", FlagName)
	}

	flags, operands := SplitArgs(rest)
	if !analyzes(flags, operands) {
		return 0, false, nil
	}

	runs, err := Plan(wd, operands)
	if err != nil {
		return 0, true, err
	}

	code, err = Execute(runs, Options{
		Exe:    exe,
		Flags:  flags,
		JSON:   hasFlag(flags, "json"),
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return 0, true, err
	}

	return code, true, nil
}

// analyzes reports whether this command line asks the driver to analyze
// packages at all.
//
// The modes that do not are the ones that must not be multiplied by the number
// of modules: printing usage, printing the -flags inventory that "go vet
// -vettool" reads to learn which flags the tool accepts, and the .cfg dispatch
// through which "go vet" hands the tool a single prepared unit of work. Running
// any of those once per module would turn one answer into several, and in the
// .cfg case would run a per-module expansion inside a driver that has already
// been told exactly which package to analyze.
//
// "go vet" can reach this: -flags advertises every flag registered on the
// global flag set, this one included, so "go vet -multi-module -vettool=qtlint"
// parses and forwards the flag to the .cfg invocation. Reaching it is harmless,
// because that invocation lands here and is passed straight through.
func analyzes(flags, operands []string) bool {
	for _, name := range []string{"flags", "h", "help"} {
		if hasFlag(flags, name) {
			return false
		}
	}

	return !isUnitConfig(operands)
}

// isUnitConfig reports whether operands are the single .cfg file that "go vet"
// passes to a vet tool, which is how the analysis driver recognizes that mode.
func isUnitConfig(operands []string) bool {
	return len(operands) == 1 && strings.HasSuffix(operands[0], ".cfg")
}

// hasFlag reports whether args set the named boolean flag to true.
func hasFlag(args []string, name string) bool {
	set := false

	for _, arg := range args {
		if arg == "--" {
			break
		}

		argName, value, isFlag := splitFlag(arg)
		if isFlag && argName == name {
			set = value != "false"
		}
	}

	return set
}
