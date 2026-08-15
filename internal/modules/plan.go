package modules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// goMod is the file whose presence makes a directory a module root.
const goMod = "go.mod"

// Run is one invocation of the command: the module directory to run it in, and
// the package patterns to give it, already written relative to that directory.
type Run struct {
	// Dir is the absolute path of the module root.
	Dir string
	// Patterns are the package patterns for this module, sorted.
	Patterns []string
}

// request is one package pattern reduced to what planning needs: the directory
// it is rooted at, and whether it sweeps the tree below that directory.
type request struct {
	root      string
	recursive bool
}

// Plan returns the runs that cover operands across every module under them,
// with operands interpreted relative to wd.
//
// The result is ordered by directory so that a run of qtlint reports modules in
// the same order every time. Two runs of the same command line then produce
// output in the same order, which is what lets one be compared to the other.
func Plan(wd string, operands []string) ([]Run, error) {
	if len(operands) == 0 {
		// The driver analyzes the current directory when given no operands.
		operands = []string{"."}
	}

	patterns := make(map[string][]string)

	for _, operand := range operands {
		if !isDirPattern(operand) {
			return nil, fmt.Errorf(
				"-%s needs directory patterns, and %q is not one: "+
					"write ./... or a path beginning with ./, ../ or /",
				FlagName, operand)
		}

		req := splitPattern(operand)

		root, err := filepath.Abs(filepath.Join(wd, filepath.FromSlash(req.root)))
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", operand, err)
		}

		req.root = root

		dirs, err := modulesUnder(req)
		if err != nil {
			return nil, err
		}

		for _, dir := range dirs {
			pattern, err := patternFor(dir, req)
			if err != nil {
				return nil, err
			}

			patterns[dir] = append(patterns[dir], pattern)
		}
	}

	if len(patterns) == 0 {
		return nil, fmt.Errorf(
			"no module found for %s: expected a %s in one of those directories or above them",
			strings.Join(quoteAll(operands), ", "), goMod)
	}

	runs := make([]Run, 0, len(patterns))
	for dir, dirPatterns := range patterns {
		slices.Sort(dirPatterns)
		runs = append(runs, Run{Dir: dir, Patterns: slices.Compact(dirPatterns)})
	}

	slices.SortFunc(runs, func(a, b Run) int { return strings.Compare(a.Dir, b.Dir) })

	return runs, nil
}

// splitPattern separates the directory part of a package pattern from the "..."
// that makes it recursive.
func splitPattern(pattern string) request {
	cleaned := filepath.ToSlash(pattern)

	switch {
	case cleaned == "...":
		return request{root: ".", recursive: true}
	case strings.HasSuffix(cleaned, "/..."):
		return request{root: strings.TrimSuffix(cleaned, "/..."), recursive: true}
	default:
		return request{root: cleaned}
	}
}

// modulesUnder returns the module directories a request reaches.
//
// Two searches answer that, and both are needed. Walking up finds the module
// the pattern sits inside, which is the module a non-recursive pattern names
// and the one that owns the packages between the root and the next module below
// it. Walking down finds the modules nested under the root, which is the half
// the go command cannot express at all. A repository whose top directory holds
// no go.mod is covered by the second search alone, so it needs no module of its
// own.
func modulesUnder(req request) ([]string, error) {
	var dirs []string

	if dir, ok := containingModule(req.root); ok {
		dirs = append(dirs, dir)
	}

	if !req.recursive {
		return dirs, nil
	}

	err := filepath.WalkDir(req.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// A directory that cannot be read is reported rather than
			// skipped. Silently walking past it would drop every module
			// under it, and a module that was never analyzed must not be
			// indistinguishable from one that was analyzed and was clean.
			return err
		}

		if !entry.IsDir() {
			return nil
		}

		// The go command ignores these when expanding "...", so a pattern
		// that reached them here would analyze packages that "./..." never
		// does. The root itself is exempt: naming such a directory outright
		// is a different request from sweeping into it.
		if path != req.root && isIgnoredDir(entry.Name()) {
			return filepath.SkipDir
		}

		// The walk continues into a module it has found, because a module
		// may hold further modules and each is its own analysis.
		if isModuleRoot(path) {
			dirs = append(dirs, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search %s for modules: %w", req.root, err)
	}

	slices.Sort(dirs)

	return slices.Compact(dirs), nil
}

// isIgnoredDir reports whether the go command ignores a directory of this name
// when it expands a "..." pattern.
func isIgnoredDir(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
		name == "testdata" || name == "vendor"
}

// containingModule returns the innermost module directory at or above dir.
func containingModule(dir string) (string, bool) {
	for {
		if isModuleRoot(dir) {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

// isModuleRoot reports whether dir holds a go.mod file.
func isModuleRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, goMod))

	return err == nil && info.Mode().IsRegular()
}

// patternFor writes the part of a request that belongs to the module rooted at
// dir, relative to that module.
//
// A module found below the root receives the whole of its own tree. The module
// that contains the root receives the subtree the caller actually named, which
// is what keeps "qtlint -multi-module ./cmd/..." from widening to the whole
// module. Packages belonging to a module nested inside this one need no
// excluding: the go command already leaves them out of a pattern expanded in
// the parent.
func patternFor(dir string, req request) (string, error) {
	rel, err := filepath.Rel(dir, req.root)
	if err != nil {
		return "", fmt.Errorf("relate %s to %s: %w", req.root, dir, err)
	}

	rel = filepath.ToSlash(rel)

	// The module sits below the root, so the request covers the whole of it.
	// Only the downward search reaches such a module, and it only runs for a
	// recursive request.
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "./...", nil
	}

	// The module is the one the pattern was written in.
	if rel == "." {
		if req.recursive {
			return "./...", nil
		}

		return ".", nil
	}

	// The module contains the root, so it receives just the named subtree.
	if req.recursive {
		return "./" + rel + "/...", nil
	}

	return "./" + rel, nil
}

// quoteAll quotes each string for an error message.
func quoteAll(values []string) []string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}

	return quoted
}
