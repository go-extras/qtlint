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
}
