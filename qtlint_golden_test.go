package qtlint_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

// TestGoldenFilesCompile type-checks the result of every suggested fix.
//
// The analysistest harness compares fixed source against the .golden file
// after gofmt, and gofmt accepts a great deal that the type checker does not:
// a rewrite that leaves a variable unused, or that names one which is not in
// scope, passes TestFixes and is still worthless. This test rebuilds the
// testdata tree with every .golden file standing in for the file it belongs
// to and loads the result, so a fix that does not compile fails here.
func TestGoldenFilesCompile(t *testing.T) {
	staged := t.TempDir()
	golden := stageGoldenTree(t, filepath.Join(analysistest.TestData(), "src"), filepath.Join(staged, "src"))
	if golden == 0 {
		t.Fatal("no .golden files were staged; the test would prove nothing")
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir:   filepath.Join(staged, "src"),
		Env:   append(os.Environ(), "GOPATH="+staged, "GO111MODULE=off", "GOWORK=off"),
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("load staged testdata: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages loaded from the staged testdata")
	}

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			t.Errorf("%s: %v", pkg.PkgPath, e)
		}
	})
}

// stageGoldenTree copies the tree rooted at src into dst, substituting the
// content of x.go.golden for x.go wherever a golden file exists. It returns
// the number of substitutions made.
func stageGoldenTree(t *testing.T, src, dst string) int {
	t.Helper()

	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatalf("create staging root: %v", err)
	}
	srcRoot, err := os.OpenRoot(src)
	if err != nil {
		t.Fatalf("open testdata root: %v", err)
	}
	t.Cleanup(func() { _ = srcRoot.Close() })
	dstRoot, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatalf("open staging root: %v", err)
	}
	t.Cleanup(func() { _ = dstRoot.Close() })

	var golden int
	err = fs.WalkDir(srcRoot.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case rel == ".":
			return nil
		case d.IsDir():
			return dstRoot.MkdirAll(rel, 0o750)
		case strings.HasSuffix(rel, ".golden"):
			return nil
		}

		source := rel
		if _, err := fs.Stat(srcRoot.FS(), rel+".golden"); err == nil {
			source = rel + ".golden"
			golden++
		}
		content, err := srcRoot.ReadFile(source)
		if err != nil {
			return err
		}
		return dstRoot.WriteFile(rel, content, 0o600)
	})
	if err != nil {
		t.Fatalf("stage testdata: %v", err)
	}
	return golden
}
