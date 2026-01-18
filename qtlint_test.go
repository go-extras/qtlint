package qtlint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/go-extras/qtlint"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()

	t.Run("basic patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "a")
	})

	t.Run("method calls", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "b")
	})

	t.Run("allowed patterns", func(t *testing.T) {
		analyzer := qtlint.NewAnalyzer()
		analysistest.Run(t, testdata, analyzer, "c")
	})
}
