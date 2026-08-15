package qtlint_test

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/go-extras/qtlint"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()

	t.Run("basic patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "a")
	})

	// A table-row function field whose every row holds one assertion, and the
	// three shapes that are not it: a row doing two things, a row calling a
	// helper, and a table of one row.
	t.Run("data rows", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-data-rows")
		analysistest.Run(t, testdata, analyzer, "datarows")
	})

	t.Run("method calls", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "b")
	})

	t.Run("allowed patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "c")
	})

	t.Run("haslen patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "haslen")
	})

	t.Run("equality istrue patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "eqistrue")
	})

	t.Run("nil comparison patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "nilcmp")
	})

	t.Run("err != nil with t.Fatal/t.Error", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "errcheck")
	})

	t.Run("strings.Contains and slices.Contains patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "strcontains")
	})

	t.Run("Contains patterns with package aliases", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "aliascontains")
	})

	t.Run("errors.Is and errors.As patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "erroris")
	})

	t.Run("errors.Is and errors.As patterns with package aliases", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "aliaserrors")
	})

	t.Run("qt.Equals with nil patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "equalsnil")
	})

	t.Run("require-qt-c-receiver patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-qt-c-receiver")
		analysistest.Run(t, testdata, analyzer, "qtcreceiver")
	})

	// The flag gate's control: the same calls, with no want comments, run
	// through an analyzer that was never told to enable the rule.
	t.Run("require-qt-c-receiver is off by default", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "qtcreceiveroff")
	})

	t.Run("require-testing-run patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		results := analysistest.Run(t, testdata, analyzer, "testingrun")
		assertReportedOnce(t, results)
	})

	t.Run("require-testing-run is off by default", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "testingrunoff")
	})

	// Both opt-in rules at once, which no other test turns on together, plus a
	// default-set diagnostic between them. The want comments cover the
	// combined path; the order assertion covers what the want comments cannot,
	// since analysistest matches each want by position and never looks at the
	// sequence the diagnostics arrived in.
	t.Run("both opt-in rules", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-qt-c-receiver")
		setFlag(t, analyzer, "require-testing-run")
		results := analysistest.Run(t, testdata, analyzer, "bothrules")
		assertReportedOrder(t, results, []string{
			"bothrules.go:29:2: qtlint: use c.Assert(...) instead of qt.Assert(t, ...)",
			"bothrules.go:31:2: qtlint: use t.Run with a per-subtest qt.New instead of c.Run",
			"bothrules.go:32:12: qtlint: use qt.HasLen instead of len(x), qt.Equals",
			"bothrules.go:35:2: qtlint: use c.Check(...) instead of qt.Check(t, ...)",
		})
	})
}

// assertReportedOnce fails when a pass reported the same message at the same
// position more than once.
//
// -require-testing-run plans one outermost function at a time, and planning
// anything smaller as well would plan the sites inside a closure twice over.
// Nothing else in the suite can see that: analysistest matches diagnostics
// against want comments by position and a second one at a position that
// already matched is simply matched again, the text driver prints one line per
// position, and -fix applies each edit set once because the two are identical.
// The count is what tells them apart, and -json is where a user would see it.
func assertReportedOnce(t *testing.T, results []*analysistest.Result) {
	t.Helper()

	for _, res := range results {
		seen := make(map[string]int)
		for _, diag := range res.Diagnostics {
			seen[fmt.Sprintf("%s: %s", res.Pass.Fset.Position(diag.Pos), diag.Message)]++
		}
		for _, key := range slices.Sorted(maps.Keys(seen)) {
			if seen[key] > 1 {
				t.Errorf("reported %d times, want once: %s", seen[key], key)
			}
		}
	}
}

// assertReportedOrder checks the sequence of diagnostics a pass reported,
// which is the sequence a driver prints: neither singlechecker nor the -json
// encoder sorts, so the analyzer's report order is the user's read order.
//
// The positions are rendered relative to the file's base name so that the
// expectation reads like the output a user sees.
func assertReportedOrder(t *testing.T, results []*analysistest.Result, want []string) {
	t.Helper()

	var got []string
	for _, res := range results {
		for _, diag := range res.Diagnostics {
			posn := res.Pass.Fset.Position(diag.Pos)
			got = append(got, fmt.Sprintf("%s:%d:%d: %s",
				filepath.Base(posn.Filename), posn.Line, posn.Column, diag.Message))
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics reported out of order:\ngot:\n\t%s\nwant:\n\t%s",
			strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// setFlag enables a boolean analyzer flag, failing the test if the flag does
// not exist.
func setFlag(t *testing.T, analyzer *analysis.Analyzer, name string) {
	t.Helper()
	if err := analyzer.Flags.Set(name, "true"); err != nil {
		t.Fatalf("set flag %s: %v", name, err)
	}
}
