package qtlint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/go-extras/qtlint"
)

func TestFixes(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "fix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "haslenfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "eqistruefix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "nilcmpfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "strcontainsfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "aliascontainsfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "errorisfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "aliaserrorsfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "equalsnilfix")

	// Default behavior: stable AND unstable errnil-fatal fixes apply.
	t.Run("errcheckfix default applies all", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "errcheckfix")
	})

	// With --only-stable-fixes: unstable fixes are withheld; diagnostics still fire.
	t.Run("errcheckfix only-stable-fixes", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		if err := analyzer.Flags.Set("only-stable-fixes", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "errcheckonlystable")
	})

	t.Run("qtcreceiverfix", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-qt-c-receiver")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "qtcreceiverfix")
	})

	// Nothing this rule rewrites can change runtime behavior, so
	// --only-stable-fixes withholds nothing: the same golden must hold.
	t.Run("qtcreceiverfix only-stable-fixes", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-qt-c-receiver")
		setFlag(t, analyzer, "only-stable-fixes")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "qtcreceiverfix")
	})

	t.Run("qtcreceiveralias", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-qt-c-receiver")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "qtcreceiveralias")
	})

	t.Run("testingrunfix", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "testingrunfix")
	})

	t.Run("testingrunalias", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "testingrunalias")
	})

	// A site whose receiver is bound further out than the closure it sits in
	// writes that receiver's name across the closures in between, so those
	// closures must not be given parameters that hide it.
	t.Run("testingrunacross", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "testingrunacross")
	})

	// Every name the rewrite writes has to still mean what the import list
	// says where it is written, or the site is declined.
	t.Run("testingrunshadow", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "testingrunshadow")
	})

	// With --only-stable-fixes: a closure using a test-scoped *qt.C method
	// keeps its diagnostic and loses its fix, and the receiver it shares with
	// a rewritten sibling keeps its declaration.
	t.Run("testingrun only-stable-fixes", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		setFlag(t, analyzer, "require-testing-run")
		setFlag(t, analyzer, "only-stable-fixes")
		analysistest.RunWithSuggestedFixes(t, testdata, analyzer, "testingrunonlystable")
	})
}
