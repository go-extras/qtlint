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
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "errcheckfix")
	analysistest.RunWithSuggestedFixes(t, testdata, qtlint.Analyzer, "errcheckfmtalias")
}
