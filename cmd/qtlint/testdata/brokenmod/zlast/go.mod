// A third module, named so that it sorts after the module that cannot load.
// That ordering is the point: the precedence rule is only observable when a
// module with diagnostics runs after a module that failed, so a rule that
// simply let the newest result win would still be caught.
module qtlint.test/zlast

go 1.21

require github.com/frankban/quicktest v0.0.0

replace github.com/frankban/quicktest => ../quicktest
