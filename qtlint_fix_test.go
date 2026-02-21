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

	// errcheck fixes require the opt-in flag.
	errFixAnalyzer := qtlint.NewAnalyzer()
	if err := errFixAnalyzer.Flags.Set("fix-err-nil", "true"); err != nil {
		t.Fatalf("failed to set fix-err-nil flag: %v", err)
	}
	analysistest.RunWithSuggestedFixes(t, testdata, errFixAnalyzer, "errcheckfix")
	analysistest.RunWithSuggestedFixes(t, testdata, errFixAnalyzer, "errcheckfmtalias")
}
