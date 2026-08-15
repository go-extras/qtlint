# qtlint

[![CI/CD Pipeline](https://github.com/go-extras/qtlint/actions/workflows/ci.yml/badge.svg)](https://github.com/go-extras/qtlint/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-extras/qtlint)](https://goreportcard.com/report/github.com/go-extras/qtlint)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

`qtlint` is a static analysis tool designed to enforce best practices for using the [frankban/quicktest](https://github.com/frankban/quicktest) testing library in Go. It is intended to be used as a **custom linter for golangci-lint**.

## Purpose

The tool helps enforce best practices for quicktest usage by detecting suboptimal patterns and suggesting better alternatives:

- Detecting `qt.Not(qt.IsNil)` and suggesting `qt.IsNotNil`
- Detecting `qt.Not(qt.IsTrue)` and suggesting `qt.IsFalse`
- Detecting `qt.Not(qt.IsFalse)` and suggesting `qt.IsTrue`
- Detecting `len(x), qt.Equals` and suggesting `x, qt.HasLen`
- Detecting `len(x), qt.Not(qt.Equals)` and suggesting `x, qt.Not(qt.HasLen)`
- Detecting `x == y, qt.IsTrue` and suggesting `x, qt.Equals, y`
- Detecting `x == y, qt.IsFalse` and suggesting `x, qt.Not(qt.Equals), y`
- Detecting `x != y, qt.IsTrue` and suggesting `x, qt.Not(qt.Equals), y`
- Detecting `x != y, qt.IsFalse` and suggesting `x, qt.Equals, y`
- Detecting `x == nil, qt.IsTrue` and suggesting `x, qt.IsNil`
- Detecting `x == nil, qt.IsFalse` and suggesting `x, qt.IsNotNil`
- Detecting `x != nil, qt.IsTrue` and suggesting `x, qt.IsNotNil`
- Detecting `x != nil, qt.IsFalse` and suggesting `x, qt.IsNil`
- Detecting `strings.Contains(x, y), qt.IsTrue` and suggesting `x, qt.Contains, y`
- Detecting `strings.Contains(x, y), qt.IsFalse` and suggesting `x, qt.Not(qt.Contains), y`
- Detecting `slices.Contains(x, y), qt.IsTrue` and suggesting `x, qt.Contains, y`
- Detecting `slices.Contains(x, y), qt.IsFalse` and suggesting `x, qt.Not(qt.Contains), y`
- Detecting `errors.Is(err, target), qt.IsTrue` and suggesting `err, qt.ErrorIs, target`
- Detecting `errors.Is(err, target), qt.IsFalse` and suggesting `err, qt.Not(qt.ErrorIs), target`
- Detecting `errors.As(err, &target), qt.IsTrue` and suggesting `err, qt.ErrorAs, &target`
- Detecting `errors.As(err, &target), qt.IsFalse` and suggesting `err, qt.Not(qt.ErrorAs), &target`
- Detecting `if err != nil { t.Fatal[f](...) }` and suggesting `c.Assert(err, qt.IsNil, qt.Commentf(...))`
- Detecting `if err != nil { t.Error[f](...) }` and suggesting `c.Check(err, qt.IsNil, qt.Commentf(...))`
- Detecting `x, qt.Equals, nil` and suggesting `x, qt.IsNil`

This ensures that tests use the most direct and readable checker available.

A second, smaller group of rules is **opt-in and off by default**. They choose between two forms that are both correct quicktest, so whether a project wants them enforced is a house-style decision rather than a correctness one:

- `-require-qt-c-receiver`: detecting `qt.Assert(t, …)` / `qt.Check(t, …)` and suggesting `c.Assert(…)` / `c.Check(…)` on a `*qt.C`
- `-require-testing-run`: detecting `c.Run(name, func(c *qt.C))` and suggesting `t.Run(name, func(t *testing.T))` with a per-subtest `qt.New`

Nothing in the default rule set changes when these flags are absent.

## Installation

### As a golangci-lint plugin

```bash
go get github.com/go-extras/qtlint
```

### As a standalone tool

```bash
# Install latest release
go install github.com/go-extras/qtlint/cmd/qtlint@latest

# Or build locally
make build

# Or install locally
make install
```

### As a pinned tool dependency (Go 1.24+)

Use the `tool` directive to keep `qtlint` versioned in your module graph for reproducible runs without a separate install step (same pattern used by `swag`).

```go
// go.mod
tool github.com/go-extras/qtlint/cmd/qtlint
```

```bash
# Pin a specific version once
go get -tool github.com/go-extras/qtlint/cmd/qtlint@vX.Y.Z

# Invoke from anywhere in the module
go tool qtlint ./...
go tool qtlint -fix ./...
```

## Usage

### Standalone Mode

Run the linter directly on your code:

```bash
# Analyze current package
qtlint .

# Analyze all packages recursively
qtlint ./...

# Auto-fix issues
qtlint -fix ./...

# Show diff without applying fixes
qtlint -fix -diff ./...

# Only apply fixes the linter is confident about (skip best-effort rewrites)
qtlint -fix -only-stable-fixes ./...

# Enable an opt-in house-style rule (off unless asked for)
qtlint -require-qt-c-receiver ./...
qtlint -fix -require-qt-c-receiver ./...
qtlint -fix -require-testing-run ./...

# Include packages and files behind a build constraint
qtlint -tags integration ./...
qtlint -tags integration,e2e ./...

# Cover every module in a repository, not just the one you are standing in
qtlint -multi-module ./...
qtlint -multi-module -tags integration ./...
```

### With golangci-lint

Add `qtlint` to your `.golangci.yml`:

```yaml
linters:
  enable:
    - qtlint
```

Then run with auto-fix:

```bash
golangci-lint run --fix
```

Note that multi-module repositories are a `golangci-lint` concern in this mode rather than a qtlint one: `golangci-lint` runs inside a single module and has no equivalent of `-multi-module` ([golangci-lint#828](https://github.com/golangci/golangci-lint/issues/828) is open at the time of writing), so it is invoked once per module — by hand, or with `automatic-module-directories` in the official GitHub Action. Use the standalone command with `-multi-module` if you want one invocation to cover the whole repository.

### `-tags` flag

Go source behind a build constraint is not part of the default build, so `qtlint ./...` does not see it. Pass `-tags` to name the constraints to satisfy, exactly as you would to `go build`:

```bash
qtlint -tags integration ./...        # one tag
qtlint -tags integration,e2e ./...    # several, comma separated
qtlint -tags 'integration e2e' ./...  # the older space separated spelling
```

Both multi-value spellings are accepted because the `go` command accepts both, and a repeated `-tags` replaces the previous value rather than adding to it — again matching `go build`.

This matters most on a recursive pattern. A package whose files are all excluded by a build constraint is not an error, it is simply absent, so without `-tags` the command reports nothing about it and still exits 0. Tagged test files sitting next to untagged source disappear the same way, and just as quietly.

Note for anyone who reached for `go vet` instead: `go vet -tags integration -vettool=$(which qtlint) ./...` has always worked, because in that mode `go vet` loads the packages itself. It is still supported and reports the same diagnostics; you no longer need it just to get build tags.

One limitation: `-tags` is forwarded through `GOFLAGS` to the `go` command that loads the packages. A custom `GOPACKAGESDRIVER` does not run the `go` command and so will not see it.

### `-multi-module` flag

`qtlint ./...` analyzes exactly one module: the one holding the working directory. Packages in any other module are not reported as missing, they are simply not there, so a repository of several modules can be linted for months while most of it is never inspected.

This is not a qtlint limitation but a `go` one. `go/packages` resolves patterns by running the `go` command, and the `go` command resolves them against the main module, so a pattern naming a different module is an error rather than a wider search:

```console
$ qtlint ./testkit/...
pattern ./testkit/...: directory prefix testkit does not contain main module or its selected dependencies
```

No spelling avoids it — an absolute path reports the same error, and naming the directory without `...` reports that the main module does not contain that package. A Go workspace does not fix it either: with a `go.work` listing every module, `./testkit/...` starts working, but `./...` still matches only the module you are standing in, so you are still the one enumerating modules.

Pass `-multi-module` and one invocation covers them all:

```bash
qtlint -multi-module ./...              # every module under the current directory
qtlint -multi-module ./services/...     # every module under services/
qtlint -multi-module -fix ./...         # fixes apply in each module
```

The flag finds every `go.mod` at or under the directories you named, then runs the linter once per module with the working directory set to it. Modules are found rather than listed, so a module added to the repository next month is covered without anyone remembering to add it.

Directories the `go` command ignores when expanding `...` are ignored here too: `vendor`, `testdata`, and any directory whose name begins with `.` or `_`. A repository whose top level holds no `go.mod` at all works — every module comes from the downward search.

**Exit codes** are the driver's own, aggregated so that the worst news wins:

| Exit | Meaning |
| ---- | ------- |
| `0`  | every module was analyzed and none reported anything |
| `3`  | some module reported diagnostics |
| other | some module could not be analyzed — a load or configuration failure |

A module that fails to load outranks one with diagnostics, because its packages were never inspected and calling that a mere finding would describe work that did not happen. A clean module can never bring the invocation back to `0`. As without the flag, `-json` reports diagnostics in the document and still exits `0`.

**Paths in diagnostics are absolute**, which is what they already were without the flag. Relative paths would be the ambiguous choice here rather than the friendly one: each module is analyzed from its own working directory, so `sub/thing_test.go` would mean a different file depending on which module produced it. `golangci-lint` reached the same conclusion from the other direction — it reports relative paths and added an absolute-path mode ([`output.path-mode: abs`](https://golangci-lint.run/docs/configuration/file/)) precisely because users running it across several projects could not tell which project a finding came from.

**`-tags` composes with it**, and both are worth running:

```bash
qtlint -multi-module ./...
qtlint -multi-module -tags integration ./...
```

Two invocations, not one, and deliberately so. A tag does not widen a build, it selects a different one: satisfying `integration` pulls in the files behind `//go:build integration` and at the same time drops those behind `//go:build !integration`. So neither run is a superset of the other, and running only the tagged one leaves the `!integration` files unlinted. qtlint does not guess which sets of tags your repository means — an unsatisfied constraint is indistinguishable from a nonexistent one, so a tool inventing tag combinations would analyze builds you never ship. Naming the contours is yours; naming the modules is not.

`-json` output is a single document however many modules ran, so anything parsing it does not need to know. Everything else the driver does is unchanged: `-fix`, `-diff`, `-c`, `-only-stable-fixes`, the opt-in rules, and the `-flags` inventory that `go vet -vettool` reads.

Two limitations. The mode needs directory patterns — `./...`, `./x/...`, or a path beginning with `./`, `../` or `/` — and refuses import paths and words like `all`, because those name packages through the module graph and carry no directory to search. And `go vet -vettool=qtlint` does its own package loading, so it still reaches one module; run qtlint directly for the rest.

### `-only-stable-fixes` flag

Some rewrites have a clear, semantically equivalent target (e.g. `qt.Not(qt.IsNil)` → `qt.IsNotNil`). Others are best-effort: rules 9 and 10 (`if err != nil { t.Fatal/Error[f](...) }`) sometimes synthesize a `qt.Commentf` from arguments that were originally joined by `Sprintln`, or pass through a format string that the linter cannot prove is a string literal. Such rewrites usually do the right thing but may change the failure-message text.

Pass `-only-stable-fixes` to withhold auto-fixes for those uncertain cases. The diagnostic still fires so you can review and apply the change by hand; only the auto-applicable fix is held back. All other rules continue to provide fixes as before.

## Rules

Rules 1 to 11 are on by default. Rules 12 and 13 are **house-style rules, off by default**, and each is named after the flag that turns it on.

All rules support **automatic fixing** with the `-fix` flag. For rules 9 and 10 the rewrite is best-effort in some variants (multi-arg `t.Fatal`, non-literal format string, `if`-init statement, spread arguments); the unsafe-by-default variants are still emitted as fixes but can be skipped with `-only-stable-fixes`. Cases that cannot be rewritten at all (init-statement and spread args) remain report-only.

### 1. Use `qt.IsNotNil` instead of `qt.Not(qt.IsNil)`

The quicktest library provides `qt.IsNotNil` as a direct checker for non-nil values, which is more readable than using `qt.Not(qt.IsNil)`.

**Bad:**
```go
c.Assert(got, qt.Not(qt.IsNil))
qt.Assert(t, got, qt.Not(qt.IsNil))
```

**Good:**
```go
c.Assert(got, qt.IsNotNil)
qt.Assert(t, got, qt.IsNotNil)
```

**Auto-fix:** ✅ Automatically replaces `qt.Not(qt.IsNil)` with `qt.IsNotNil`

**Error message:**
```
qtlint: use qt.IsNotNil instead of qt.Not(qt.IsNil)
```

### 2. Use `qt.IsFalse` instead of `qt.Not(qt.IsTrue)`

**Bad:**
```go
c.Assert(value, qt.Not(qt.IsTrue))
```

**Good:**
```go
c.Assert(value, qt.IsFalse)
```

**Auto-fix:** ✅ Automatically replaces `qt.Not(qt.IsTrue)` with `qt.IsFalse`

**Error message:**
```
qtlint: use qt.IsFalse instead of qt.Not(qt.IsTrue)
```

### 3. Use `qt.IsTrue` instead of `qt.Not(qt.IsFalse)`

**Bad:**
```go
c.Assert(value, qt.Not(qt.IsFalse))
```

**Good:**
```go
c.Assert(value, qt.IsTrue)
```

**Auto-fix:** ✅ Automatically replaces `qt.Not(qt.IsFalse)` with `qt.IsTrue`

**Error message:**
```
qtlint: use qt.IsTrue instead of qt.Not(qt.IsFalse)
```

### 4. Use `qt.HasLen` / `qt.Not(qt.HasLen)` instead of `len(x), qt.Equals` / `len(x), qt.Not(qt.Equals)`

The quicktest library provides `qt.HasLen` as a direct checker for checking the length of slices, arrays, maps, and strings, which is more readable than using `len(x), qt.Equals` or `len(x), qt.Not(qt.Equals)`.

**Bad:**
```go
c.Assert(len(mySlice), qt.Equals, 3)
qt.Assert(t, len(myMap), qt.Equals, 5)
c.Assert(len(events), qt.Not(qt.Equals), 0)
qt.Assert(t, len(events), qt.Not(qt.Equals), 0)
```

**Good:**
```go
c.Assert(mySlice, qt.HasLen, 3)
qt.Assert(t, myMap, qt.HasLen, 5)
c.Assert(events, qt.Not(qt.HasLen), 0)
qt.Assert(t, events, qt.Not(qt.HasLen), 0)
```

**Auto-fix:** ✅ Automatically replaces `len(x), qt.Equals` with `x, qt.HasLen` and `len(x), qt.Not(qt.Equals)` with `x, qt.Not(qt.HasLen)`

**Error messages:**
```
qtlint: use qt.HasLen instead of len(x), qt.Equals
qtlint: use qt.Not(qt.HasLen) instead of len(x), qt.Not(qt.Equals)
```

### 5. Use `qt.Equals` / `qt.Not(qt.Equals)` instead of equality comparisons with `qt.IsTrue` / `qt.IsFalse`

Equality and inequality comparisons embedded in the "got" argument should use the appropriate checker directly.

**Bad:**
```go
c.Assert(x == y, qt.IsTrue)
c.Assert(x == y, qt.IsFalse)
c.Assert(x != y, qt.IsTrue)
c.Assert(x != y, qt.IsFalse)
```

**Good:**
```go
c.Assert(x, qt.Equals, y)
c.Assert(x, qt.Not(qt.Equals), y)
c.Assert(x, qt.Not(qt.Equals), y)
c.Assert(x, qt.Equals, y)
```

**Auto-fix:** ✅ Automatically replaces with the appropriate `qt.Equals` or `qt.Not(qt.Equals)` checker

**Error messages:**
```
qtlint: use qt.Equals instead of x == y, qt.IsTrue
qtlint: use qt.Not(qt.Equals) instead of x == y, qt.IsFalse
qtlint: use qt.Not(qt.Equals) instead of x != y, qt.IsTrue
qtlint: use qt.Equals instead of x != y, qt.IsFalse
```

### 6. Use `qt.IsNil`/`qt.IsNotNil` instead of nil comparison with `qt.IsTrue`/`qt.IsFalse`

Nil comparisons embedded in the "got" argument should use the dedicated `qt.IsNil` or `qt.IsNotNil` checkers.

**Bad:**
```go
c.Assert(x == nil, qt.IsTrue)
c.Assert(x == nil, qt.IsFalse)
c.Assert(x != nil, qt.IsTrue)
c.Assert(x != nil, qt.IsFalse)
```

**Good:**
```go
c.Assert(x, qt.IsNil)
c.Assert(x, qt.IsNotNil)
c.Assert(x, qt.IsNotNil)
c.Assert(x, qt.IsNil)
```

**Auto-fix:** ✅ Automatically replaces with the appropriate `qt.IsNil` or `qt.IsNotNil` checker

**Error messages:**
```
qtlint: use qt.IsNil instead of x == nil, qt.IsTrue
qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse
qtlint: use qt.IsNotNil instead of x != nil, qt.IsTrue
qtlint: use qt.IsNil instead of x != nil, qt.IsFalse
```

### 7. Use `qt.Contains` instead of `strings.Contains(x, y)` or `slices.Contains(x, y)` with `qt.IsTrue`/`qt.IsFalse`

The quicktest library provides `qt.Contains` as a direct checker for checking if a string, slice, array, or map contains a value. This is more readable than using `strings.Contains` or `slices.Contains` with `qt.IsTrue` or `qt.IsFalse`.

**Bad:**
```go
c.Assert(strings.Contains(str, "world"), qt.IsTrue)
qt.Assert(t, strings.Contains(str, "foo"), qt.IsFalse)
c.Assert(slices.Contains(slice, 42), qt.IsTrue)
qt.Assert(t, slices.Contains(slice, 99), qt.IsFalse)
```

**Good:**
```go
c.Assert(str, qt.Contains, "world")
qt.Assert(t, str, qt.Not(qt.Contains), "foo")
c.Assert(slice, qt.Contains, 42)
qt.Assert(t, slice, qt.Not(qt.Contains), 99)
```

**Auto-fix:** ✅ Automatically replaces `strings.Contains(x, y), qt.IsTrue` with `x, qt.Contains, y`, `strings.Contains(x, y), qt.IsFalse` with `x, qt.Not(qt.Contains), y`, and similarly for `slices.Contains`

**Error message:**
```
qtlint: use qt.Contains instead of strings.Contains(x, y), qt.IsTrue
qtlint: use qt.Not(qt.Contains) instead of strings.Contains(x, y), qt.IsFalse
qtlint: use qt.Contains instead of slices.Contains(x, y), qt.IsTrue
qtlint: use qt.Not(qt.Contains) instead of slices.Contains(x, y), qt.IsFalse
```

### 8. Use `qt.ErrorIs` / `qt.ErrorAs` instead of `errors.Is(...)` / `errors.As(...)` with `qt.IsTrue` / `qt.IsFalse`

The quicktest library provides `qt.ErrorIs` and `qt.ErrorAs` as direct checkers for `errors.Is` and `errors.As`. Wrapping those calls in `qt.IsTrue`/`qt.IsFalse` hides the intent and produces less informative failure messages.

**Bad:**
```go
c.Assert(errors.Is(err, services.ErrClosedLoanFieldImmutable), qt.IsTrue)
qt.Assert(t, errors.Is(err, fs.ErrNotExist), qt.IsFalse)
c.Assert(errors.As(err, &target), qt.IsTrue)
qt.Assert(t, errors.As(err, &target), qt.IsFalse)
```

**Good:**
```go
c.Assert(err, qt.ErrorIs, services.ErrClosedLoanFieldImmutable)
qt.Assert(t, err, qt.Not(qt.ErrorIs), fs.ErrNotExist)
c.Assert(err, qt.ErrorAs, &target)
qt.Assert(t, err, qt.Not(qt.ErrorAs), &target)
```

**Auto-fix:** ✅ Automatically replaces `errors.Is(err, target), qt.IsTrue` with `err, qt.ErrorIs, target` (and the `qt.IsFalse` / `errors.As` variants in the same way)

**Error messages:**
```
qtlint: use qt.ErrorIs instead of errors.Is(err, target), qt.IsTrue
qtlint: use qt.Not(qt.ErrorIs) instead of errors.Is(err, target), qt.IsFalse
qtlint: use qt.ErrorAs instead of errors.As(err, target), qt.IsTrue
qtlint: use qt.Not(qt.ErrorAs) instead of errors.As(err, target), qt.IsFalse
```

### 9. Use `c.Assert(err, qt.IsNil)` instead of `if err != nil { t.Fatal[f](...) }`

When a `*qt.C` variable and a quicktest import are in scope, the pattern `if err != nil { t.Fatal(...) }` should be replaced with a `c.Assert` call.

**Bad:**
```go
if err != nil {
    t.Fatal(err)
}
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
```

**Good:**
```go
c.Assert(err, qt.IsNil)
c.Assert(err, qt.IsNil, qt.Commentf("unexpected error: %v", err))
```

**Auto-fix:** ✅ for `t.Fatal(err)`, `t.Fatal()`, and `t.Fatalf(literal, …)` with a string-literal format. Best-effort (suppressed by `-only-stable-fixes`) for `t.Fatal("msg:", err, 123)` (multi-arg; format is synthesized as `"%v %v %v"`) and `t.Fatalf(formatVar, …)` (non-literal format). Not provided for `if err := f(); err != nil { t.Fatal(…) }` (init statement would change scoping) or for `t.Fatal(args...)` (spread arguments are opaque).

**Error message:**
```
qtlint: use c.Assert(err, qt.IsNil) instead of t.Fatal(...)
qtlint: use c.Assert(err, qt.IsNil, qt.Commentf(...)) instead of t.Fatalf(...)
```

### 10. Use `c.Check(err, qt.IsNil)` instead of `if err != nil { t.Error[f](...) }`

Same as rule 9, but for `t.Error`/`t.Errorf` which maps to `c.Check` (non-fatal assertion).

**Bad:**
```go
if err != nil {
    t.Error(err)
}
if err != nil {
    t.Errorf("unexpected error: %v", err)
}
```

**Good:**
```go
c.Check(err, qt.IsNil)
c.Check(err, qt.IsNil, qt.Commentf("unexpected error: %v", err))
```

**Auto-fix:** Same stability matrix as rule 9 (above), targeting `c.Check` instead of `c.Assert`.

**Error message:**
```
qtlint: use c.Check(err, qt.IsNil) instead of t.Error(...)
qtlint: use c.Check(err, qt.IsNil, qt.Commentf(...)) instead of t.Errorf(...)
```

### 11. Use `qt.IsNil` instead of `qt.Equals, nil`

The quicktest `Equals` checker compares `got` and `want` with `==`. A typed nil (e.g. `(*T)(nil)`) never equals the untyped `nil` literal, so `c.Assert((*T)(nil), qt.Equals, nil)` fails at runtime; only an untyped nil interface happens to pass. quicktest's own documentation recommends `qt.IsNil` for nil checks.

**Bad:**
```go
c.Assert(x, qt.Equals, nil)
qt.Assert(t, x, qt.Equals, nil)
```

**Good:**
```go
c.Assert(x, qt.IsNil)
qt.Assert(t, x, qt.IsNil)
```

**Auto-fix:** ✅ Automatically replaces `qt.Equals, nil` with `qt.IsNil`, dropping the `want` argument. Trailing arguments such as `qt.Commentf(...)` are preserved.

**Error message:**
```
qtlint: use qt.IsNil instead of qt.Equals, nil
```

### 12. Require assertions to go through a `*qt.C` receiver — `-require-qt-c-receiver`

**House-style rule, off by default.** quicktest exposes both a package-level assertion taking a `testing.TB` and a method on `*qt.C`, and both are correct. Some projects require the second form everywhere, so that a test function has exactly one `*qt.C` and every assertion goes through it: the `*qt.C` is what carries `c.Cleanup`, `c.Setenv`, `c.TempDir`, `c.Patch`, `c.Defer` and `c.Parallel`, as well as any comment state the test attached to it, and a file that mixes both forms grows two ways of reaching the test's context. Pass `-require-qt-c-receiver` to enforce it; without the flag nothing below is reported.

**Bad:**
```go
func TestExample(t *testing.T) {
    qt.Assert(t, got, qt.Equals, want)
    qt.Check(t, err, qt.IsNil)
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    c := qt.New(t)
    c.Assert(got, qt.Equals, want)
    c.Check(err, qt.IsNil)
}
```

**Auto-fix:** ✅ for every reported call, and `-only-stable-fixes` withholds none of them — creating a `*qt.C` from a `*testing.T` cannot change what the test does. The fix reuses a `*qt.C` that was created from the same `*testing.T` when one is visible under its own name at the call site, and otherwise inserts `c := qt.New(t)` as the first statement of the function that *binds* that `*testing.T` — the subtest closure rather than the parent test when the assertion sits inside one, and the helper's own body when the assertion sits in a helper taking `t *testing.T`. When the name `c` is already taken in that function, the next free name (`c2`, `c3`, …) is used rather than declining the fix.

The rule does not fire when:

- the first argument is not an identifier of type `*testing.T` — `qt.Assert` accepts any `testing.TB`, and a `*testing.B`, a bare `testing.TB` or a field selector such as `h.t` is left alone;
- no function on the enclosing stack binds that identifier as a parameter, so there is nowhere to put the `qt.New` call.

An aliased quicktest import is matched the same way the default rules match it — through the type checker, not the identifier — and the fix writes whichever alias the file uses.

**Error message:**
```
qtlint: use c.Assert(...) instead of qt.Assert(t, ...)
qtlint: use c.Check(...) instead of qt.Check(t, ...)
```

### 13. Require `t.Run` with a per-subtest `qt.New` — `-require-testing-run`

**House-style rule, off by default.** `c.Run` is a legitimate quicktest API and some projects prefer it, so nothing below is reported unless you pass `-require-testing-run`.

Projects that enforce the standard-library form do it for three reasons. The subtest signature stops being special: `t.Run(name, func(t *testing.T))` is what every Go reader and every tool already expects, so table-driven helpers and anything taking a `*testing.T` compose without a shim. Shadowing becomes visible: `c.Run(name, func(c *qt.C))` shadows the outer `c` with a different `*qt.C` of the same name and type, and a reader cannot tell from the body which one a line means. And the parent's `*qt.C` stops being reachable by accident — under `c.Run` a closure can still name the *enclosing* `c` for a `Cleanup` or a `Patch`, which binds to the parent test rather than the subtest, and that is more often a mistake than an intention.

**Bad:**
```go
func TestExample(t *testing.T) {
    c := qt.New(t)
    c.Run("sub", func(c *qt.C) {
        c.Assert(got, qt.Equals, want)
    })
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    t.Run("sub", func(t *testing.T) {
        c := qt.New(t)
        c.Assert(got, qt.Equals, want)
    })
}
```

Note that this repository's own tests are written in the target form. Enabling this rule while leaving a `c.Run` example in the project's own style guide is how the rule gets argued with six months later, so a project adopting it should update its contributor documentation at the same time — most `quicktest` examples in the wild show `c.Run`.

**Auto-fix:** ✅ for every reported site except the ones noted below, and the rewrite touches four things at once so that the result compiles:

- the receiver of `.Run` becomes the `*testing.T` the `*qt.C` was made from;
- the closure parameter becomes `t *testing.T`, keeping the original `*qt.C` identifier for the body to use;
- `c := qt.New(t)` opens the closure — but only when the closure still needs a `*qt.C` after the rewrite. A closure whose sole use of it was the receiver of a nested `c.Run` loses that use, and a declaration with no uses does not compile;
- the receiver's own `c := qt.New(t)` is removed when the rewrite takes its last use, for the same reason.

The new parameter is named `t` unless the closure already refers to something called `t` — usually the enclosing test — in which case the next free name is used and that reference keeps meaning what it meant.

**Nested subtests** are rewritten as one consistent set of edits. The inner call names the parameter the outer rewrite introduces, which is resolved before the inner one is planned; applying an outer rewrite on its own would otherwise leave an inner `c.Run` whose receiver no longer exists.

**A receiver bound further out than the closure the call sits in** takes one more precaution. The rewrite writes that receiver's name *across* the closures in between, and those closures are being given parameters by the same plan, so a parameter introduced in between would hide the one that was meant:

```go
c.Run("outer", func(c *qt.C) {
    c.Run("middle", func(mid *qt.C) { // renames its own C, so c below still means "outer"
        mid.Assert(0, qt.Equals, 0)
        c.Run("deep", func(c *qt.C) { /* ... */ })
    })
})
```

Naming every closure's parameter `t` here leaves `deep` reading the *middle* subtest's `t`. The result compiles and passes, and the only thing that changes is that the subtest moves from `outer/deep` to `outer/middle/deep`, which breaks every `-run` filter and anything else keyed on test names. So a closure that a name is written across is kept clear of that name, and only such a closure is: a plain nest wants the shadowing, since that is what lets each level of `t.Run(name, func(t *testing.T))` call itself `t`. The name written across may come from the plan — an enclosing closure's new parameter — or from the source, as the `t` behind a `c := qt.New(t)` declared outside the closure in between; both are kept clear of.

**A whole function is planned before any of it is reported**, because its sites decide each other's fate. Whether the receiver's declaration survives depends on which of its `c.Run` calls are actually rewritten, so a call the rule declined — one whose `t` is shadowed where the rewrite would write it, say — keeps that declaration alive for every sibling. And a nested call is declined whenever the call around it is, since rewriting it alone would attach the subtest to the parent test instead. Answering "which sites get edits" and "does the declaration survive" separately is how a declaration comes to be deleted while a declined sibling still names it.

**`Defer` and `Done` are withheld whatever the flags say.** They are quicktest's deferred-execution API, and they are the one shape where this rewrite can turn a passing test into a panicking one. `(*C).Defer` registers a cleanup that panics unless `Done` has run first; `C.Run` wraps the closure it calls in `defer c2.Done()`, a bare `c := qt.New(t)` does not, and nothing in the rewritten closure would. Measured against `quicktest v1.14.6`: a subtest calling `c.Defer` passes under `c.Run` and panics with `Done not called after Defer` under `t.Run` plus `qt.New`. The diagnostic still fires; the fix does not.

**`-only-stable-fixes`** additionally withholds the fix — the diagnostic still fires — when the closure calls `Cleanup`, `Parallel`, `Setenv`, `Unsetenv`, `TempDir`, `Patch`, `Mkdir`, `Chdir` or `Context` on its own `*qt.C`. That is a review gate, not a correctness one: `C.Run` builds the closure's `*qt.C` from the subtest's own `*testing.T`, so both forms reach the same test, and measured against `quicktest v1.14.6` all of them behave identically either way — `Setenv`, `Unsetenv` and `Patch` restore at the same point, `Cleanup` runs at the same point, `TempDir` and `Mkdir` name the same subtest-scoped directory, and `Chdir` and `Context` bind to the subtest. What they have in common is that they tie a subtest to a test's *lifecycle* rather than to an assertion, which is where a reader has to agree that the subtest is the scope that was meant, so the flag lets a project migrate them by hand.

The last two are worth naming separately: `*qt.C` embeds `testing.TB`, so `Chdir` and `Context` — added to that interface in Go 1.24 — are as callable on a `*qt.C` as `Setenv` is, without appearing among `C`'s own declarations. An inventory of what a `*qt.C` can do has to follow the embedding, and `testing.TB` gains methods with the language.

Both sets are matched by asking **what can be called on the closure's `*qt.C`**, not what is written next to it. The `*qt.C` is followed through plain assignments — `cc := c`, `var cc = c`, and on — and any use the rule cannot follow, such as `helper(c)` or `holder{c: c}`, is treated as reaching everything. That is what stops `cc := c; cc.Defer(...)` from shipping the panic through a route the method names never see; the cost is that a closure handing its `*qt.C` to a helper loses its automatic fix even when the helper does nothing interesting with it.

When a fix is withheld for either reason, so are the fixes for any subtests nested inside it, and the receiver's declaration is kept, because the `c.Run` that was left alone still uses it.

The rule does not fire when:

- **the subtest is a named function rather than a literal.** Its signature is `func(*qt.C)`, which `t.Run` will not accept; rewriting it means changing a declaration that may have callers elsewhere in the package or beyond it. That is out of scope, and since a reported site with no fix puts the work back on the author for a rewrite the tool declined to reason about, such a call is not reported at all;
- **the receiver is not traceable to a `*testing.T`** — a `*qt.C` that arrived as a parameter, came out of a struct field or a factory, or was assigned again after `qt.New`. There is no name the rewrite could put in front of `.Run`;
- **the file does not import `testing` under a usable name**, since the new parameter type has to name it;
- **the receiver's declaration would have to go but cannot be removed cleanly** — it shares its line with other code, or carries a trailing comment the deletion would take with it. There is no correct fix for such a site, and a reported site without one puts the repair back on the author, so the rule stays quiet about it. Any subtest nested inside it is declined with it;
- **a name the rewrite would write does not mean, where it would be written, what the import list says.** The rewrite emits three names — the receiver, the `testing` qualifier in the new parameter type, and the `quicktest` qualifier in the inserted `qt.New` — and each is checked at its own insertion point. A `testing := 1` in the test function hides the package from a closure signature that never mentioned it; a file that imports `quicktest` twice can have the first spelling shadowed where the second one still resolves. The input compiles in both cases, because only the rewrite introduces the reference.

Both packages are resolved through the type checker, so an aliased `quicktest` or `testing` import is matched and the rewrite writes whichever names the file uses.

**Error message:**
```
qtlint: use t.Run with a per-subtest qt.New instead of c.Run
```

## Examples

The linter works with both package-level functions and method calls:

```go
import qt "github.com/frankban/quicktest"

func TestExample(t *testing.T) {
    c := qt.New(t)
    
    // Package-level function
    qt.Assert(t, value, qt.Not(qt.IsNil))  // ❌ Will be flagged
    qt.Assert(t, value, qt.IsNotNil)       // ✅ Correct
    
    // Method call
    c.Assert(value, qt.Not(qt.IsNil))      // ❌ Will be flagged
    c.Assert(value, qt.IsNotNil)           // ✅ Correct
    
    // qt.Not(qt.Equals) with a plain value is allowed
    c.Assert(value, qt.Not(qt.Equals), 42) // ✅ Correct
    // but qt.Not(qt.Equals) with len() is flagged:
    c.Assert(len(value), qt.Not(qt.Equals), 0) // ❌ Will be flagged
    c.Assert(value, qt.Not(qt.HasLen), 0)      // ✅ Correct
}
```

## Development

The project includes a Makefile for common development tasks:

```bash
# Build the standalone binary
make build

# Install to GOPATH/bin
make install

# Run tests
make test

# Run linter
make lint

# Run formatters (auto-fix)
make fmt

# Clean build artifacts
make clean

# Show all available targets
make help
```

Or use Go commands directly:

```bash
# Run tests
go test ./...

# Build all packages
go build ./...

# Build standalone binary
go build -o bin/qtlint ./cmd/qtlint

# Test GoReleaser configuration
goreleaser check

# Build snapshot (local testing)
goreleaser build --snapshot --clean --single-target
```

### Releases

Releases are automated using GoReleaser:

- **Pull Requests**: Snapshot builds are created as artifacts for testing
- **Tagged Releases**: Production releases are published to GitHub Releases when a tag is pushed

To create a new release:

```bash
# Tag the release
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag
git push origin v1.0.0
```

The CI/CD pipeline will automatically:
- Build binaries for all supported platforms (Linux, macOS, Windows, FreeBSD)
- Create archives (tar.gz for Unix, zip for Windows)
- Generate checksums
- Publish to GitHub Releases

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

