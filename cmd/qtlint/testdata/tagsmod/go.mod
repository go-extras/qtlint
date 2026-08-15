// This module is a fixture for the -tags end-to-end test. It lives under
// testdata so the go tool never walks into it from the qtlint module, and it
// replaces quicktest with a local stub so it resolves with no network and no
// go.sum.
module qtlint.test/tagsmod

go 1.21

require github.com/frankban/quicktest v0.0.0

replace github.com/frankban/quicktest => ./quicktest
