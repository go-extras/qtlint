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
- Detecting `x == y, qt.IsTrue` and suggesting `x, qt.Equals, y`
- Detecting `x == y, qt.IsFalse` and suggesting `x, qt.Not(qt.Equals), y`
- Detecting `x != y, qt.IsTrue` and suggesting `x, qt.Not(qt.Equals), y`
- Detecting `x != y, qt.IsFalse` and suggesting `x, qt.Equals, y`
- Detecting `x == nil, qt.IsTrue` and suggesting `x, qt.IsNil`
- Detecting `x == nil, qt.IsFalse` and suggesting `x, qt.IsNotNil`
- Detecting `x != nil, qt.IsTrue` and suggesting `x, qt.IsNotNil`
- Detecting `x != nil, qt.IsFalse` and suggesting `x, qt.IsNil`
- Detecting `if err != nil { t.Fatal[f](...) }` and suggesting `c.Assert(err, qt.IsNil, qt.Commentf(...))`
- Detecting `if err != nil { t.Error[f](...) }` and suggesting `c.Check(err, qt.IsNil, qt.Commentf(...))`

This ensures that tests use the most direct and readable checker available.

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

## Rules

All rules support **automatic fixing** with the `-fix` flag.

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

### 4. Use `qt.HasLen` instead of `len(x), qt.Equals`

The quicktest library provides `qt.HasLen` as a direct checker for checking the length of slices, arrays, maps, and strings, which is more readable than using `len(x), qt.Equals`.

**Bad:**
```go
c.Assert(len(mySlice), qt.Equals, 3)
qt.Assert(t, len(myMap), qt.Equals, 5)
```

**Good:**
```go
c.Assert(mySlice, qt.HasLen, 3)
qt.Assert(t, myMap, qt.HasLen, 5)
```

**Auto-fix:** ✅ Automatically replaces `len(x), qt.Equals` with `x, qt.HasLen`

**Error message:**
```
qtlint: use qt.HasLen instead of len(x), qt.Equals
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
    
    // qt.Not with other checkers is allowed
    c.Assert(value, qt.Not(qt.Equals), 42) // ✅ Allowed
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

