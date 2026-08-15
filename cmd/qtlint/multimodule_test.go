// Multi-module tests for the qtlint command.
//
// Like the -tags tests next to them, these drive the built binary: the wiring
// under test decides which working directory the analysis driver loads packages
// from, and nothing short of running the command can see that. The fixture
// module in testdata/multimod holds three modules, two of them nested inside
// the first, so that a single invocation has something to cover that no
// package pattern can reach.
package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// multimodDir is the multi-module fixture, relative to this package.
const multimodDir = "testdata/multimod"

// brokenmodDir is the fixture for exit-code precedence: an outer module with a
// violation, and a module nested inside it that does not type-check.
const brokenmodDir = "testdata/brokenmod"

// The violations planted in the fixture, as fixture-relative paths.
//
// Each names the contour it belongs to. Two of them are the point of the
// fixture: tagged is in the build only when the integration tag is set, and
// untagged only when it is not, because a build tag adds files to a build and
// never removes any. No single run reports both.
const (
	root     = "root/root_test.go"           // no constraint, every run
	untagged = "contour/plain_test.go"       // //go:build !integration
	tagged   = "contour/integration_test.go" // //go:build integration
	nested   = "nested/sub/sub_test.go"      // second module, no constraint
	quiet    = "quiet/hush/hush_test.go"     // third module, //go:build integration
)

// runMultimod runs the command in the multi-module fixture.
func runMultimod(t *testing.T, args ...string) result {
	t.Helper()

	return runCommand(t, fixturePath(t, multimodDir), nil, qtlintBin, args...)
}

// TestMultiModuleReachesEveryModule is the test that separates an invocation
// covering one module from one covering all of them. Every row states the files
// the go command puts in the build for that spelling, across every module the
// pattern reaches.
func TestMultiModuleReachesEveryModule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
		code int
	}{{
		// The acceptance control. Without the flag the command still sees
		// one module, so every row below is a statement about the flag
		// rather than about the fixture.
		name: "without the flag only the module holding the pattern is seen",
		args: []string{"./..."},
		want: []string{untagged, root},
		code: 3,
	}, {
		name: "the flag reaches the modules nested inside that one",
		args: []string{"-multi-module", "./..."},
		want: []string{untagged, root, nested},
		code: 3,
	}, {
		name: "build tags reach every module the flag found",
		args: []string{"-multi-module", "-tags", "integration", "./..."},
		want: []string{tagged, root, nested, quiet},
		code: 3,
	}, {
		// A pattern naming one nested module covers that module alone.
		// The outer module's violations must not come back with it.
		name: "a pattern naming a nested module covers only that module",
		args: []string{"-multi-module", "./nested/..."},
		want: []string{nested},
		code: 3,
	}, {
		// ...and a pattern naming a subtree of the outer module must not
		// widen to the whole of it.
		name: "a pattern naming a subtree keeps that subtree",
		args: []string{"-multi-module", "./root/..."},
		want: []string{root},
		code: 3,
	}, {
		name: "a clean module reports nothing and exits 0",
		args: []string{"-multi-module", "./quiet/..."},
		want: nil,
		code: 0,
	}, {
		// Several patterns are a union, and a module named twice is run
		// once rather than reported twice.
		name: "several patterns are covered together",
		args: []string{"-multi-module", "./nested/...", "./root/..."},
		want: []string{root, nested},
		code: 3,
	}, {
		name: "the flag can be turned off the way the flag package spells it",
		args: []string{"-multi-module=false", "./..."},
		want: []string{untagged, root},
		code: 3,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runMultimod(t, tc.args...)

			want := slices.Clone(tc.want)
			slices.Sort(want)

			if !slices.Equal(got.files, want) {
				t.Errorf("reported files\ngot:  %q\nwant: %q\noutput:\n%s", got.files, want, got.output)
			}
			if got.code != tc.code {
				t.Errorf("exit code = %d, want %d; output:\n%s", got.code, tc.code, got.output)
			}
		})
	}
}

// TestNeitherContourAloneReportsEverything pins why a tagged run cannot stand
// in for a plain one, whatever the module coverage is.
//
// A build tag only ever adds files to a build. Satisfying "integration" brings
// the tagged file in and takes the "!integration" file out, so each run has a
// violation the other cannot see, and only the two together report all five.
// Without this, a change that quietly ran everything with the tag set would
// still satisfy every other test here.
func TestNeitherContourAloneReportsEverything(t *testing.T) {
	t.Parallel()

	plain := runMultimod(t, "-multi-module", "./...")
	integration := runMultimod(t, "-multi-module", "-tags", "integration", "./...")

	if slices.Contains(plain.files, tagged) {
		t.Errorf("the plain run reported %s, which is behind the tag:\n%s", tagged, plain.output)
	}
	if slices.Contains(integration.files, untagged) {
		t.Errorf("the tagged run reported %s, which the tag excludes:\n%s", untagged, integration.output)
	}

	union := slices.Concat(plain.files, integration.files)
	slices.Sort(union)
	union = slices.Compact(union)

	want := []string{tagged, untagged, nested, quiet, root}
	slices.Sort(want)

	if !slices.Equal(union, want) {
		t.Errorf("the two runs together\ngot:  %q\nwant: %q", union, want)
	}
}

// TestTaggedOnlyModuleDoesNotPassSilently pins the failure shape this tool has
// hidden twice.
//
// The third fixture module's only violation sits behind a build constraint. A
// package excluded by a constraint is not an error, it is simply absent, so a
// run that never reached the module and a run that reached it and found nothing
// produce the same silence and the same exit code. This asks for the tag and
// requires both a report and a non-zero exit, which is the only pair the two
// cases do not share.
func TestTaggedOnlyModuleDoesNotPassSilently(t *testing.T) {
	t.Parallel()

	got := runMultimod(t, "-multi-module", "-tags", "integration", "./quiet/...")

	if !slices.Contains(got.files, quiet) {
		t.Errorf("reported nothing behind the tag; output:\n%s", got.output)
	}
	if got.code == 0 {
		t.Errorf("exit code = 0 with a violation behind the tag; output:\n%s", got.output)
	}
}

// TestFindingsInAnyModuleOutrankACleanOne pins the exit code, which is what a
// CI job reads. A module with no findings must not carry the invocation to 0
// when another module has them, whichever order they run in.
func TestFindingsInAnyModuleOutrankACleanOne(t *testing.T) {
	t.Parallel()

	// The quiet module is clean without the tag, and sorts after the two
	// modules that are not, so it is the last word on the exit code.
	got := runMultimod(t, "-multi-module", "./...")

	if got.code == 0 {
		t.Errorf("exit code = 0 with findings in other modules; output:\n%s", got.output)
	}
	if got.code != 3 {
		t.Errorf("exit code = %d, want the driver's own 3 for diagnostics; output:\n%s", got.code, got.output)
	}
}

// TestALoadFailureOutranksDiagnostics pins the other half of the exit code.
//
// A module the driver could not analyze is worse news than a module with
// diagnostics: its packages were never inspected, and reporting that as a
// finding would describe work that did not happen. The two controls are what
// make the third line mean anything — without them a rule that always returned
// 1, or one that returned whatever the last module gave, would satisfy it.
func TestALoadFailureOutranksDiagnostics(t *testing.T) {
	t.Parallel()

	dir := fixturePath(t, brokenmodDir)

	good := runCommand(t, dir, nil, qtlintBin, "-multi-module", "./good/...")
	if good.code != 3 {
		t.Fatalf("the module with a violation exited %d, want 3; output:\n%s", good.code, good.output)
	}

	broken := runCommand(t, dir, nil, qtlintBin, "-multi-module", "./broken/...")
	if broken.code != 1 {
		t.Fatalf("the module that cannot load exited %d, want 1; output:\n%s", broken.code, broken.output)
	}

	both := runCommand(t, dir, nil, qtlintBin, "-multi-module", "./...")
	if both.code != 1 {
		t.Errorf("together they exited %d, want 1: a module that was never analyzed "+
			"must not be reported as a mere finding; output:\n%s", both.code, both.output)
	}

	// The failing module must not have ended the run either. The caller asked
	// about every module, and stopping early would hide the findings a later
	// one was about to report.
	if len(both.files) == 0 {
		t.Errorf("no diagnostics reported, so the failing module stopped the run; output:\n%s", both.output)
	}
}

// TestJSONIsOneDocument checks that -json survives the change.
//
// The driver prints one JSON object per run, and a caller parsing qtlint's
// output must not have to know that several processes produced it. Two objects
// written one after another are not a JSON document at all, so this parses the
// output strictly and requires packages from more than one module inside it.
func TestJSONIsOneDocument(t *testing.T) {
	t.Parallel()

	got := runMultimod(t, "-multi-module", "-json", "./...")

	var tree map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got.stdout), &tree); err != nil {
		t.Fatalf("output is not one JSON document: %v\ngot:\n%s", err, got.stdout)
	}

	var modules []string
	for pkg := range tree {
		module, _, _ := strings.Cut(pkg, " ")
		modules = append(modules, module)
	}

	for _, want := range []string{"qtlint.test/multimod/contour", "qtlint.test/nested/sub"} {
		if !slices.Contains(modules, want) {
			t.Errorf("package %q missing from the combined document; got %q", want, modules)
		}
	}

	// The driver exits 0 in -json mode even with diagnostics, reporting
	// them in the document instead. That is preserved rather than corrected.
	if got.code != 0 {
		t.Errorf("exit code = %d, want 0 as the driver gives in -json mode; output:\n%s", got.code, got.output)
	}
}

// TestFixAppliesInEveryModule checks that -fix survives the change, which is
// one of the things replacing the analysis driver would have cost.
func TestFixAppliesInEveryModule(t *testing.T) {
	t.Parallel()

	// -fix rewrites files, so it runs against a copy rather than against
	// the fixture every other test reads.
	//
	// The symlinks are resolved because the driver reports the resolved path
	// of every file. On macOS the temporary directory is reached through
	// /var, a symlink to /private/var, so an unresolved directory would make
	// every diagnostic look like it came from outside the tree and this test
	// would pass while reading nothing.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}

	copyTree(t, fixturePath(t, multimodDir), dir)

	got := runCommand(t, dir, nil, qtlintBin, "-multi-module", "./...")
	if len(got.files) == 0 {
		t.Fatalf("nothing to fix, so this proves nothing; output:\n%s", got.output)
	}

	if fixed := runCommand(t, dir, nil, qtlintBin, "-multi-module", "-fix", "./..."); fixed.code != 0 {
		t.Fatalf("fix run exited %d; output:\n%s", fixed.code, fixed.output)
	}

	after := runCommand(t, dir, nil, qtlintBin, "-multi-module", "./...")
	if len(after.files) != 0 {
		t.Errorf("still reported after -fix: %q\noutput:\n%s", after.files, after.output)
	}

	// The nested module is the one no pattern can reach, so it is the one
	// worth reading back.
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(nested)))
	if err != nil {
		t.Fatalf("read %s: %v", nested, err)
	}
	if !strings.Contains(string(body), "qt.IsNotNil") {
		t.Errorf("%s was not rewritten:\n%s", nested, body)
	}
}

// TestFlagInventoryIsPrintedOnce checks the mode "go vet -vettool" depends on.
//
// -flags is how go vet learns which flags the tool accepts, and it advertises
// every flag on the global set, this one included. So go vet can be asked for
// -multi-module, and the flag must not turn one inventory into one per module.
func TestFlagInventoryIsPrintedOnce(t *testing.T) {
	t.Parallel()

	// The pattern is recursive on purpose. Without it the plan holds one
	// module, one inventory is printed either way, and this would pass just
	// as happily if -flags were not exempt from the expansion at all.
	got := runMultimod(t, "-multi-module", "-flags", "./...")

	var flags []struct {
		Name string
		Bool bool
	}
	if err := json.Unmarshal([]byte(got.stdout), &flags); err != nil {
		t.Fatalf("-flags did not print one JSON document: %v\ngot:\n%s", err, got.stdout)
	}

	var names []string
	for _, f := range flags {
		names = append(names, f.Name)
	}

	if !slices.Contains(names, "multi-module") {
		t.Errorf("-flags does not advertise the flag; got %q", names)
	}
}

// TestVetToolStillWorks checks that the flag has not disturbed the .cfg
// dispatch, the mode that makes qtlint usable as a vet tool at all. The vet
// command does its own package loading, so it reaches one module, and that is
// the answer it should still give.
func TestVetToolStillWorks(t *testing.T) {
	t.Parallel()

	dir := fixturePath(t, multimodDir)

	want := []string{untagged, root}
	slices.Sort(want)

	vet := runCommand(t, dir, nil, "go", "vet", "-vettool="+qtlintBin, "./...")
	if !slices.Equal(vet.files, want) {
		t.Errorf("go vet -vettool reported\ngot:  %q\nwant: %q\noutput:\n%s", vet.files, want, vet.output)
	}

	// go vet learns the tool's flags from -flags, which advertises this one,
	// so it accepts -multi-module and forwards it to the .cfg invocation. That
	// invocation has already been told exactly which package to analyze, and
	// must pass straight through rather than plan an expansion of its own.
	// Without the .cfg exemption this run fails outright.
	flagged := runCommand(t, dir, nil, "go", "vet", "-multi-module", "-vettool="+qtlintBin, "./...")
	if !slices.Equal(flagged.files, want) {
		t.Errorf("go vet -multi-module -vettool reported\ngot:  %q\nwant: %q\noutput:\n%s",
			flagged.files, want, flagged.output)
	}
}

// TestRefusesPatternsItCannotResolve pins the refusal.
//
// Multi-module mode turns a pattern into directories to search for go.mod
// files, and an import path or a reserved word such as "all" carries no
// directory. Analyzing whatever subset happened to resolve and exiting 0 is the
// shape of a run that looks clean because it inspected nothing, so such a
// pattern is refused instead.
func TestRefusesPatternsItCannotResolve(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"all", "qtlint.test/multimod/..."} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			got := runMultimod(t, "-multi-module", pattern)

			if got.code == 0 {
				t.Errorf("exit code = 0 for %q; output:\n%s", pattern, got.output)
			}
			if !strings.Contains(got.output, "directory patterns") {
				t.Errorf("refusal does not say what is wrong; output:\n%s", got.output)
			}
		})
	}
}

// TestHelpDescribesTheFlag checks that -h says the mode exists. A capability
// nobody can discover is one nobody uses.
func TestHelpDescribesTheFlag(t *testing.T) {
	t.Parallel()

	got := runMultimod(t, "-h")

	if !strings.Contains(got.output, "-multi-module") {
		t.Errorf("help does not mention the flag:\n%s", got.output)
	}
}

// copyTree copies the fixture into dir so that a -fix run has something of its
// own to rewrite.
func copyTree(t *testing.T, from, to string) {
	t.Helper()

	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatalf("copy %s: %v", from, err)
	}
}
