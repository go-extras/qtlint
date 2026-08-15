package modules

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
)

// Options describe how Execute runs the plan.
type Options struct {
	// Exe is the command to re-run, normally the running binary.
	Exe string
	// Flags are the arguments to give every child, with the multi-module
	// flag already removed and the package operands not yet appended.
	Flags []string
	// JSON reports whether -json was requested, which changes both what a
	// child's standard output means and how the results combine.
	JSON bool
	// Stdout and Stderr are where a child's output goes.
	Stdout io.Writer
	Stderr io.Writer
}

// Exit codes of the analysis driver, established by running it.
//
// The driver reports diagnostics with 3 and a package it could not analyze with
// 1, and it is careful to distinguish them. Aggregating has to preserve that
// distinction rather than flattening every non-zero result into one.
const (
	exitClean       = 0
	exitDiagnostics = 3
)

// Execute runs each entry of the plan and returns the exit code for the whole
// invocation.
//
// The code is the worst news any module produced. A module the driver could not
// analyze outranks a module with diagnostics, because a failed load means the
// packages were never inspected and reporting that as a mere finding would
// describe work that did not happen. Diagnostics in turn outrank a clean
// module, so a single clean module can never carry the invocation to 0. This is
// the property that makes a multi-module run usable in CI at all.
//
// Exit codes are otherwise the driver's own, unchanged: 0 when every module is
// clean, 3 when some module reported diagnostics, and whatever the driver
// returned when it failed. In -json mode the driver itself exits 0 even with
// diagnostics, reporting them in the document instead, and that is preserved
// here rather than corrected.
func Execute(runs []Run, opts Options) (int, error) {
	merged := make(tree)

	worst := exitClean

	for _, run := range runs {
		code, out, err := execute(run, opts)
		if err != nil {
			return 0, err
		}

		if opts.JSON {
			if err := merged.add(out); err != nil {
				return 0, fmt.Errorf("module %s: %w", run.Dir, err)
			}
		}

		switch {
		case code == exitClean:
		case code == exitDiagnostics && worst == exitClean:
			worst = exitDiagnostics
		case code != exitDiagnostics:
			// A driver failure ends the aggregation as the answer, but
			// every module still runs: the caller asked about all of
			// them, and stopping early would hide findings that a later
			// module was about to report.
			if worst == exitClean || worst == exitDiagnostics {
				worst = code
			}
		}
	}

	if opts.JSON {
		if err := merged.print(opts.Stdout); err != nil {
			return 0, err
		}
	}

	return worst, nil
}

// execute runs one module and returns its exit code, plus its standard output
// when that output is a JSON document this package has to combine.
//
// Outside -json mode the child writes straight through to the parent's streams.
// The driver prints diagnostics to standard error and JSON to standard output,
// so nothing is buffered that a caller might be watching: a long multi-module
// run reports each module as it finishes rather than at the end.
func execute(run Run, opts Options) (code int, stdout []byte, err error) {
	args := append(slices.Clone(opts.Flags), run.Patterns...)

	// The command is this program and the arguments are the ones it was
	// given, with the module's own patterns in place of the caller's. Neither
	// is attacker-controlled in any sense the caller does not already have:
	// anyone who can choose them can run the binary directly.
	//nolint:gosec // re-running this same binary is the mechanism, not a risk
	cmd := exec.Command(opts.Exe, args...)
	cmd.Dir = run.Dir
	cmd.Env = append(os.Environ(), childEnv+"=1")
	cmd.Stderr = opts.Stderr

	var buf bytes.Buffer
	if opts.JSON {
		cmd.Stdout = &buf
	} else {
		cmd.Stdout = opts.Stdout
	}

	var exitErr *exec.ExitError

	switch err := cmd.Run(); {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		return 0, nil, fmt.Errorf("run in %s: %w", run.Dir, err)
	}

	return code, buf.Bytes(), nil
}
