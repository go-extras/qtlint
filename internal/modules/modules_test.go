// Package modules_test exercises the multi-module planning through the
// package's exported surface only. What the command does with a plan is an
// end-to-end question, and is pinned next to the command in
// cmd/qtlint/multimodule_test.go by running the built binary; what is here is
// the part that can be decided without a go command: which arguments are
// package operands, and which modules a set of patterns reaches.
package modules_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-extras/qtlint/internal/modules"
)

func TestRequested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
		on   bool
	}{{
		name: "absent",
		args: []string{"./..."},
		want: []string{"./..."},
		on:   false,
	}, {
		name: "present",
		args: []string{"-multi-module", "./..."},
		want: []string{"./..."},
		on:   true,
	}, {
		name: "two leading dashes",
		args: []string{"--multi-module", "./..."},
		want: []string{"./..."},
		on:   true,
	}, {
		name: "set to true",
		args: []string{"-multi-module=true", "./..."},
		want: []string{"./..."},
		on:   true,
	}, {
		// The flag package reads -x=false as false, and a mode that
		// ignored that would be impossible to turn off from a wrapper
		// script that always passes the flag.
		name: "set to false",
		args: []string{"-multi-module=false", "./..."},
		want: []string{"./..."},
		on:   false,
	}, {
		name: "a repeat takes the last value",
		args: []string{"-multi-module", "-multi-module=false", "./..."},
		want: []string{"./..."},
		on:   false,
	}, {
		// Other flags have to survive, because they are what the child
		// is run with.
		name: "other flags are kept in order",
		args: []string{"-c", "0", "-multi-module", "-tags", "integration", "./..."},
		want: []string{"-c", "0", "-tags", "integration", "./..."},
		on:   true,
	}, {
		// After a bare "--" the flag package is no longer reading flags,
		// so this is a package pattern that happens to look like one.
		name: "after a bare double dash it is an operand",
		args: []string{"--", "-multi-module"},
		want: []string{"--", "-multi-module"},
		on:   false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, on := modules.Requested(tc.args)
			if on != tc.on {
				t.Errorf("on = %v, want %v", on, tc.on)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("rest\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		flags    []string
		operands []string
	}{{
		name:     "no flags",
		args:     []string{"./..."},
		flags:    nil,
		operands: []string{"./..."},
	}, {
		name:     "no operands",
		args:     []string{"-json"},
		flags:    []string{"-json"},
		operands: nil,
	}, {
		name:     "boolean flag does not swallow the operand",
		args:     []string{"-fix", "./..."},
		flags:    []string{"-fix"},
		operands: []string{"./..."},
	}, {
		// The shape that matters. -c takes its value as a separate
		// argument, so a split that did not know that would read "0" as a
		// package pattern and plan the whole run around it.
		name:     "a value taking flag swallows the next argument",
		args:     []string{"-c", "0", "./..."},
		flags:    []string{"-c", "0"},
		operands: []string{"./..."},
	}, {
		name:     "a value joined with equals does not swallow",
		args:     []string{"-c=0", "./..."},
		flags:    []string{"-c=0"},
		operands: []string{"./..."},
	}, {
		name:     "several operands",
		args:     []string{"-tags", "integration", "./a/...", "./b/..."},
		flags:    []string{"-tags", "integration"},
		operands: []string{"./a/...", "./b/..."},
	}, {
		// The flag package stops at "--" and drops it; keeping it with
		// the flags is what lets a command line be put back together.
		name:     "a bare double dash ends the flags",
		args:     []string{"-fix", "--", "-weird"},
		flags:    []string{"-fix", "--"},
		operands: []string{"-weird"},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flags, operands := modules.SplitArgs(tc.args)
			if !slices.Equal(flags, tc.flags) {
				t.Errorf("flags\ngot:  %q\nwant: %q", flags, tc.flags)
			}
			if !slices.Equal(operands, tc.operands) {
				t.Errorf("operands\ngot:  %q\nwant: %q", operands, tc.operands)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// wd is relative to the tree built for the test.
		wd       string
		operands []string
		// want pairs a module directory, relative to the tree, with the
		// patterns that module is to be given.
		want map[string][]string
	}{{
		name:     "a recursive pattern reaches the modules nested inside",
		operands: []string{"./..."},
		want: map[string][]string{
			".":      {"./..."},
			"nested": {"./..."},
			"quiet":  {"./..."},
		},
	}, {
		// Not recursive, so nothing is searched for below: this is the
		// single package the caller named, and it belongs to one module.
		name:     "a plain directory names one module",
		operands: []string{"."},
		want:     map[string][]string{".": {"."}},
	}, {
		name:     "no operands behave as the current directory",
		operands: nil,
		want:     map[string][]string{".": {"."}},
	}, {
		name:     "a pattern naming a nested module covers only it",
		operands: []string{"./nested/..."},
		want:     map[string][]string{"nested": {"./..."}},
	}, {
		// The subtree the caller named has to survive: widening this to
		// "./..." would analyze the whole module they narrowed away from.
		name:     "a subtree of a module keeps its subtree",
		operands: []string{"./pkg/..."},
		want:     map[string][]string{".": {"./pkg/..."}},
	}, {
		name:     "several patterns are a union",
		operands: []string{"./nested/...", "./pkg/..."},
		want: map[string][]string{
			".":      {"./pkg/..."},
			"nested": {"./..."},
		},
	}, {
		// One module reached twice is one run, not two.
		name:     "a module named twice is planned once",
		operands: []string{"./nested/...", "./nested/sub"},
		want:     map[string][]string{"nested": {"./...", "./sub"}},
	}, {
		name:     "a pattern written from inside a nested module",
		wd:       "nested",
		operands: []string{"./..."},
		want:     map[string][]string{"nested": {"./..."}},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tree := buildTree(t)

			runs, err := modules.Plan(filepath.Join(tree, tc.wd), tc.operands)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}

			got := make(map[string][]string, len(runs))
			for _, run := range runs {
				rel, err := filepath.Rel(tree, run.Dir)
				if err != nil {
					t.Fatalf("relate %s: %v", run.Dir, err)
				}

				got[filepath.ToSlash(rel)] = run.Patterns
			}

			if len(got) != len(tc.want) {
				t.Errorf("modules\ngot:  %v\nwant: %v", got, tc.want)
			}

			for dir, want := range tc.want {
				if !slices.Equal(got[dir], want) {
					t.Errorf("patterns for %s\ngot:  %q\nwant: %q", dir, got[dir], want)
				}
			}
		})
	}
}

// TestPlanSkipsDirectoriesTheGoCommandIgnores pins the walk against the go
// command's own rules for expanding "...". A module under testdata or vendor,
// or under a directory whose name starts with "." or "_", is not part of what
// "./..." means, and analyzing it would report diagnostics from files the
// caller never asked about — a linter's own fixtures, most of all.
func TestPlanSkipsDirectoriesTheGoCommandIgnores(t *testing.T) {
	t.Parallel()

	tree := buildTree(t)

	for _, dir := range []string{"testdata/mod", "vendor/mod", ".hidden/mod", "_work/mod"} {
		writeModule(t, filepath.Join(tree, filepath.FromSlash(dir)), "ignored.test/"+filepath.Base(dir))
	}

	runs, err := modules.Plan(tree, []string{"./..."})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, run := range runs {
		rel, err := filepath.Rel(tree, run.Dir)
		if err != nil {
			t.Fatalf("relate %s: %v", run.Dir, err)
		}

		for _, ignored := range []string{"testdata", "vendor", ".hidden", "_work"} {
			if strings.HasPrefix(filepath.ToSlash(rel), ignored+"/") {
				t.Errorf("planned a run in %s, which the go command ignores", rel)
			}
		}
	}
}

// TestPlanFindsModulesWithNoModuleAbove covers a repository that is a container
// of modules rather than a module. The go command cannot analyze it at all
// today — there is no main module to run in — so every module in it has to come
// from the downward search.
func TestPlanFindsModulesWithNoModuleAbove(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeModule(t, filepath.Join(tree, "alpha"), "container.test/alpha")
	writeModule(t, filepath.Join(tree, "beta"), "container.test/beta")

	runs, err := modules.Plan(tree, []string{"./..."})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var got []string
	for _, run := range runs {
		rel, err := filepath.Rel(tree, run.Dir)
		if err != nil {
			t.Fatalf("relate %s: %v", run.Dir, err)
		}

		got = append(got, filepath.ToSlash(rel))
	}

	want := []string{"alpha", "beta"}
	if !slices.Equal(got, want) {
		t.Errorf("modules\ngot:  %q\nwant: %q", got, want)
	}
}

// TestPlanRefusesPatternsWithNoDirectory pins the refusal. These patterns name
// packages through the module graph or through a reserved word, so there is no
// directory to search for modules under. Planning whatever part of them happens
// to resolve would report a subset and exit 0.
func TestPlanRefusesPatternsWithNoDirectory(t *testing.T) {
	t.Parallel()

	tree := buildTree(t)

	for _, operand := range []string{"all", "std", "example.com/x/...", "example.com/x"} {
		t.Run(operand, func(t *testing.T) {
			t.Parallel()

			if _, err := modules.Plan(tree, []string{operand}); err == nil {
				t.Errorf("Plan(%q) succeeded, want a refusal", operand)
			}
		})
	}
}

// TestPlanReportsWhenNothingIsAModule checks that an empty answer is an error
// rather than an empty plan. A plan with no runs would exit 0 having analyzed
// nothing, which is indistinguishable from a clean repository.
func TestPlanReportsWhenNothingIsAModule(t *testing.T) {
	t.Parallel()

	if _, err := modules.Plan(t.TempDir(), []string{"./..."}); err == nil {
		t.Error("Plan succeeded with no module anywhere, want a refusal")
	}
}

// TestPlanIsOrdered checks that the runs come back in a stable order. Two runs
// of the same command line have to report modules in the same order, or their
// outputs cannot be compared to each other.
func TestPlanIsOrdered(t *testing.T) {
	t.Parallel()

	tree := buildTree(t)

	first, err := modules.Plan(tree, []string{"./..."})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	second, err := modules.Plan(tree, []string{"./..."})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !slices.EqualFunc(first, second, func(a, b modules.Run) bool { return a.Dir == b.Dir }) {
		t.Errorf("two plans disagree\nfirst:  %v\nsecond: %v", first, second)
	}

	if !slices.IsSortedFunc(first, func(a, b modules.Run) int { return strings.Compare(a.Dir, b.Dir) }) {
		t.Errorf("plan is not ordered: %v", first)
	}
}

// buildTree writes the module layout the planning tests share:
//
//	.           a module, with a plain package in pkg
//	./nested    a module inside it, with a package in sub
//	./quiet     a second module inside it
func buildTree(t *testing.T) string {
	t.Helper()

	tree := t.TempDir()

	writeModule(t, tree, "tree.test/outer")
	writeModule(t, filepath.Join(tree, "nested"), "tree.test/nested")
	writeModule(t, filepath.Join(tree, "quiet"), "tree.test/quiet")

	for _, dir := range []string{"pkg", filepath.Join("nested", "sub")} {
		if err := os.MkdirAll(filepath.Join(tree, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	return tree
}

// writeModule makes dir a module root.
func writeModule(t *testing.T, dir, path string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}

	body := "module " + path + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(body), 0o600); err != nil {
		t.Fatalf("write go.mod in %s: %v", dir, err)
	}
}
