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
}
