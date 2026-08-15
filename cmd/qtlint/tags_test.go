// Package main_test drives the qtlint command as a program.
//
// The -tags wiring under test lives in main and only has an observable effect
// on the go command that the analysis driver runs to load packages, so nothing
// short of building the binary and running it against a module can see whether
// the flag arrived. The fixture module in testdata/tagsmod carries the two
// shapes a build constraint takes: a package that still loads without the tag
// while its constrained test file is dropped, and a package that disappears
// from a recursive pattern entirely.
package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fixtureDir is the module the command is run against, relative to this
// package's directory.
const fixtureDir = "testdata/tagsmod"

// qtlintBin is the command under test, built once by TestMain.
var qtlintBin string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "qtlint-tags")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)

		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	qtlintBin = filepath.Join(dir, "qtlint")
	if runtime.GOOS == "windows" {
		qtlintBin += ".exe"
	}

	if out, err := exec.Command("go", "build", "-o", qtlintBin, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build qtlint: %v\n%s", err, out)

		return 1
	}

	return m.Run()
}

// TestTagsReachesTheLoader is the test that separates a -tags flag which is
// merely accepted from one that is honored. Every row is a spelling the go
// command itself accepts, and the wanted files are the ones "go list" reports
// as belonging to the build for that spelling.
func TestTagsReachesTheLoader(t *testing.T) {
	t.Parallel()

	const (
		plain  = "plain/plain_test.go"
		quiet  = "quiet/quiet_test.go"
		hidden = "hidden/hidden_test.go"
		alpha  = "alpha/alpha_test.go"
		beta   = "beta/beta_test.go"
	)

	tests := []struct {
		name string
		env  []string
		args []string
		want []string
	}{{
		// The acceptance control. Without it every row below is satisfied by
		// a fix that simply satisfies every build constraint.
		name: "no tag leaves constrained files out of the build",
		args: []string{"./..."},
		want: []string{plain},
	}, {
		name: "space separated value",
		args: []string{"-tags", "qtprobe", "./..."},
		want: []string{hidden, plain, quiet},
	}, {
		name: "value joined with equals",
		args: []string{"-tags=qtprobe", "./..."},
		want: []string{hidden, plain, quiet},
	}, {
		name: "two leading dashes",
		args: []string{"--tags=qtprobe", "./..."},
		want: []string{hidden, plain, quiet},
	}, {
		name: "comma separated multi value",
		args: []string{"-tags", "qtalpha,qtbeta", "./..."},
		want: []string{alpha, beta, plain},
	}, {
		// The spelling go kept for compatibility with Go 1.12 and earlier.
		name: "space separated multi value",
		args: []string{"-tags", "qtalpha qtbeta", "./..."},
		want: []string{alpha, beta, plain},
	}, {
		// go's own -tags replaces on repeat rather than accumulating.
		name: "a repeated flag takes the last value",
		args: []string{"-tags", "qtalpha", "-tags", "qtbeta", "./..."},
		want: []string{beta, plain},
	}, {
		// -c takes a separate value, so a scan that stopped at the first
		// argument it could not classify would never reach -tags.
		name: "after another flag that takes a separate value",
		args: []string{"-c", "0", "-tags", "qtprobe", "./..."},
		want: []string{hidden, plain, quiet},
	}, {
		name: "a package named explicitly rather than by pattern",
		args: []string{"-tags", "qtprobe", "./hidden/"},
		want: []string{hidden},
	}, {
		// An inherited GOFLAGS must not be thrown away by a command line that
		// says nothing about tags.
		name: "no flag keeps the tags already in GOFLAGS",
		env:  []string{"GOFLAGS=-tags=qtprobe"},
		args: []string{"./..."},
		want: []string{hidden, plain, quiet},
	}, {
		// ...and must lose to a command line that does, which is the
		// precedence the go command gives an explicit flag.
		name: "the flag beats the tags already in GOFLAGS",
		env:  []string{"GOFLAGS=-tags=qtalpha"},
		args: []string{"-tags", "qtprobe", "./..."},
		want: []string{hidden, plain, quiet},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runQtlint(t, tc.env, tc.args...)
			if !slices.Equal(got.files, tc.want) {
				t.Errorf("reported files\ngot:  %q\nwant: %q\noutput:\n%s", got.files, tc.want, got.output)
			}
		})
	}
}

// TestRecursivePatternDoesNotPassSilently pins the failure shape that hid the
// defect: on a recursive pattern a package excluded by a build constraint is
// not an error, it is simply absent, so the command reports nothing and exits
// 0 having inspected nothing behind the tag.
func TestRecursivePatternDoesNotPassSilently(t *testing.T) {
	t.Parallel()

	got := runQtlint(t, nil, "-tags", "qtprobe", "./quiet/...")

	if len(got.files) == 0 {
		t.Errorf("reported nothing behind the tag; output:\n%s", got.output)
	}
	if got.code == 0 {
		t.Errorf("exit code = 0 with a violation behind the tag; output:\n%s", got.output)
	}
}

// TestMatchesVetTool checks the standalone command against the same analyzer
// driven by "go vet -vettool", which loads packages itself and has always
// applied -tags. The two must see the same files.
func TestMatchesVetTool(t *testing.T) {
	t.Parallel()

	standalone := runQtlint(t, nil, "-tags", "qtprobe", "./...")
	vet := runCommand(t, fixturePath(t, fixtureDir), nil, "go", "vet", "-tags", "qtprobe", "-vettool="+qtlintBin, "./...")

	if !slices.Equal(standalone.files, vet.files) {
		t.Errorf("standalone and vettool disagree\nstandalone: %q\nvettool:    %q", standalone.files, vet.files)
	}
	if len(vet.files) == 0 {
		t.Fatalf("vettool control reported nothing, so it proves nothing; output:\n%s", vet.output)
	}
}

// TestHelpDescribesTags checks that -h says what -tags does. The driver
// registers the flag as a deprecated shim documented as having no effect,
// which would send a reader to "go vet -vettool" for something this command
// now does on its own.
func TestHelpDescribesTags(t *testing.T) {
	t.Parallel()

	got := runQtlint(t, nil, "-h")

	const want = "  -tags string\n    \ta comma-separated list of build tags"
	if !strings.Contains(got.output, want) {
		t.Errorf("help does not describe -tags\nwant substring: %q\ngot:\n%s", want, got.output)
	}
	// Other shims keep their wording, so this looks only at the -tags entry.
	if strings.Contains(got.output, "  -tags string\n    \tno effect") {
		t.Errorf("help still calls -tags inert:\n%s", got.output)
	}
}

// result is what one run of a command reported.
type result struct {
	// files are the fixture-relative paths named by diagnostics, sorted and
	// deduplicated.
	files []string
	// code is the process exit code.
	code int
	// output is standard output followed by standard error, for
	// failure messages.
	output string
	// stdout is standard output alone. The driver prints diagnostics to
	// standard error and its JSON documents to standard output, so a test
	// about -json or -flags has to read this rather than the mixture.
	stdout string
}

func runQtlint(t *testing.T, env []string, args ...string) result {
	t.Helper()

	return runCommand(t, fixturePath(t, fixtureDir), env, qtlintBin, args...)
}

func runCommand(t *testing.T, dir string, env []string, name string, args ...string) result {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// GOFLAGS is the channel the fix writes to, so a value inherited from the
	// developer's shell would decide these results instead of the flag under
	// test. GOWORK keeps an ambient workspace from claiming the fixture
	// module. Entries appended later win, so a row's own env overrides these.
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	cmd.Env = append(cmd.Env, env...)

	// The two streams are captured separately rather than into one buffer.
	// os/exec serializes writes only when Stdout and Stderr are the same
	// comparable value; anything else, a wrapper included, gets a goroutine
	// each and they would race on the shared buffer.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	err := cmd.Run()

	output := stdout.String() + stderr.String()

	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		t.Fatalf("run %s %q: %v\n%s", name, args, err, output)
	}

	return result{
		files:  diagnosedFiles(dir, output),
		code:   code,
		output: output,
		stdout: stdout.String(),
	}
}

// diagnosedFiles extracts the fixture-relative file of every diagnostic in
// output. The standalone command prints absolute paths and go vet prints paths
// relative to the module, so both are resolved against dir.
func diagnosedFiles(dir, output string) []string {
	files := make([]string, 0)

	for _, line := range strings.Split(output, "\n") {
		const ext = ".go"

		i := strings.Index(line, ext+":")
		if i < 0 {
			continue
		}

		path := line[:i+len(ext)]
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}

		files = append(files, filepath.ToSlash(rel))
	}

	slices.Sort(files)

	return slices.Compact(files)
}

func fixturePath(t *testing.T, dir string) string {
	t.Helper()

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("locate %s: %v", dir, err)
	}

	return abs
}
