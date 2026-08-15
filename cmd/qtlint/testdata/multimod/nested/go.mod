// A module nested inside the outer module. The go command refuses to reach it
// from there by any spelling of a package pattern, which is what the
// multi-module mode exists to solve.
module qtlint.test/nested

go 1.21

require github.com/frankban/quicktest v0.0.0

replace github.com/frankban/quicktest => ../quicktest
