// A nested module that does not type-check, so the analyzer is skipped for it
// and the driver reports a load failure rather than diagnostics.
module qtlint.test/broken

go 1.21
