// The outer module of the fixture that pins exit-code precedence. It holds a
// violation, so on its own it would exit 3; the module nested inside it cannot
// be analyzed at all, and that has to be the answer instead.
module qtlint.test/brokenmod

go 1.21

require github.com/frankban/quicktest v0.0.0

replace github.com/frankban/quicktest => ./quicktest
