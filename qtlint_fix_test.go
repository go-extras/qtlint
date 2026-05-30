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
}
