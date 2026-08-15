// Package tagsflag makes the qtlint command's -tags flag real.
//
// The analysis driver qtlint is built on,
// golang.org/x/tools/go/analysis/singlechecker, registers -tags itself: not as
// a build-tag flag, but as one of a handful of inert shims kept so that scripts
// written for older releases of "go vet" keep parsing. Its registered usage
// text is literally "no effect (deprecated)", and the value it parses is
// discarded. The driver then loads packages through go/packages with a
// configuration it never exposes, so a caller of singlechecker.Main has no way
// to put the tags on packages.Config.BuildFlags.
//
// What go/packages does expose is the go command it shells out to, and the go
// command reads build flags from the GOFLAGS environment variable. Writing the
// requested tags there is therefore the one channel that reaches the loader
// without replacing the driver, and replacing the driver would cost qtlint the
// behavior that comes with it: -fix, -diff, -json, -c, the -flags inventory
// that "go vet -vettool" reads, and the .cfg dispatch that makes qtlint usable
// as a vet tool in the first place.
//
// The one input this cannot reach is a custom GOPACKAGESDRIVER, which does not
// run the go command and so never sees GOFLAGS.
package tagsflag

import (
	"bytes"
	"io"
	"strings"
)

// name is the command-line flag this package forwards.
const name = "tags"

// Forward returns the GOFLAGS value that makes the go command satisfy the build
// tags requested on the command line in args, starting from the current GOFLAGS
// value in goflags.
//
// It reports false when args carry no -tags flag at all. A caller that is given
// false must leave GOFLAGS exactly as it found it: an inherited GOFLAGS may
// already carry a -tags setting the user meant to keep, and overwriting it with
// an empty one would take away build tags that plain "qtlint ./..." honors
// today.
func Forward(args []string, goflags string) (string, bool) {
	value, ok := lastValue(args)
	if !ok {
		return "", false
	}

	entry := "-" + name + "=" + strings.Join(forwardable(Split(value)), ",")
	if goflags == "" {
		return entry, true
	}

	// The go command applies GOFLAGS entries left to right and a repeated
	// -tags replaces rather than extends, so the last entry wins. Appending
	// is what lets an explicit -tags on the qtlint command line beat an
	// inherited GOFLAGS, which is the precedence the go command itself gives
	// a flag written on the command line.
	return goflags + " " + entry, true
}

// Split splits a -tags flag value into build tags the way the go command does.
//
// A value containing a space or a quote is split on whitespace, which is the
// spelling the go command still accepts for compatibility with Go 1.12 and
// earlier. Every other value is split on commas, dropping empty entries. Both
// spellings reach the same set of tags: "go build -tags a,b" and
// "go build -tags 'a b'" are the same request.
func Split(value string) []string {
	if strings.ContainsAny(value, " \t'\"") {
		return strings.Fields(value)
	}

	tags := make([]string, 0, strings.Count(value, ",")+1)
	for _, tag := range strings.Split(value, ",") {
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	return tags
}

// forwardable drops the tags that the GOFLAGS encoding cannot carry.
//
// GOFLAGS is split on spaces and has no quoting, so the whole setting has to
// travel as one comma-separated word. A tag holding a space, a
// comma or a quote cannot survive that, and it could not have been satisfied
// either: a build constraint names its tags in the identifier alphabet, so the
// go command would have carried such a tag along and never matched it. Dropping
// it here reaches the same set of files, and keeps GOFLAGS free of the quotes
// the go command would not have unquoted anyway.
func forwardable(tags []string) []string {
	kept := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !strings.ContainsAny(tag, " \t,'\"") {
			kept = append(kept, tag)
		}
	}

	return kept
}

// lastValue returns the value of the last -tags flag in args, matching the go
// command's rule that a repeated -tags replaces the previous setting rather
// than adding to it.
//
// The scan is deliberately blind to how many arguments each flag takes. The
// driver's flag set is the authority on that, and it does not exist until the
// driver parses, which is after the point where GOFLAGS still matters. So every
// argument up to a bare "--" is examined. A package pattern never begins with a
// dash, so the only argument this can misread is a literal "-tags" written as
// the value of some other flag.
func lastValue(args []string) (string, bool) {
	var (
		value string
		found bool
	)

	for i := 0; i < len(args); i++ {
		// The flag package stops parsing flags at a bare "--", so a -tags
		// after it is a package pattern rather than a flag.
		if args[i] == "--" {
			break
		}

		trimmed, ok := trimDashes(args[i])
		if !ok {
			continue
		}

		flagName, inline, hasInline := strings.Cut(trimmed, "=")
		if flagName != name {
			continue
		}

		if hasInline {
			value, found = inline, true

			continue
		}

		// A trailing "-tags" with nothing after it is an error the driver
		// reports for us; there is nothing to forward.
		if i+1 < len(args) {
			value, found = args[i+1], true
			i++
		}
	}

	return value, found
}

// trimDashes strips the leading dashes from arg and reports whether arg is
// shaped like a flag at all. The flag package accepts one dash or two, and
// treats "-", "--" and anything not starting with a dash as something other
// than a flag.
func trimDashes(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", false
	}

	trimmed := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if trimmed == "" || strings.HasPrefix(trimmed, "-") {
		return "", false
	}

	return trimmed, true
}

// driverUsageHead is how the flag package renders the first line of the
// driver's -tags shim: two spaces, the flag name, and the value placeholder.
const driverUsageHead = "  -" + name + " string\n"

// driverUsageMark appears in the shim's usage text and in no honest -tags
// description, so requiring it keeps UsageWriter from rewriting a future
// release of x/tools that has made -tags real by itself.
const driverUsageMark = "no effect"

// usageText is what -tags does in this command.
const usageText = driverUsageHead +
	"    \ta comma-separated list of build tags to consider satisfied, as with go build -tags\n"

// UsageWriter returns a writer that forwards to w, correcting the -tags usage
// line the analysis driver prints.
//
// The driver registers -tags as a deprecated shim and describes it as having no
// effect. That description is wrong for this command, and a user who reads it
// would reasonably conclude the flag is not worth passing. The flag package
// renders each flag's usage with a single Write to the flag set's output, which
// is what makes this correction possible; flag.CommandLine.SetOutput is the
// supported way to redirect that output.
//
// If a future release of x/tools words the shim differently, or the flag
// package stops writing one flag at a time, no line matches and the driver's
// own text is printed unchanged. Everything else written to w is passed through
// untouched.
func UsageWriter(w io.Writer) io.Writer {
	return &usageWriter{w: w}
}

type usageWriter struct {
	w io.Writer
}

func (u *usageWriter) Write(p []byte) (int, error) {
	if !bytes.HasPrefix(p, []byte(driverUsageHead)) || !bytes.Contains(p, []byte(driverUsageMark)) {
		return u.w.Write(p)
	}

	if _, err := u.w.Write([]byte(usageText)); err != nil {
		return 0, err
	}

	// Report the caller's own length: it wrote p, and a short count would
	// read as an io.ErrShortWrite that never happened.
	return len(p), nil
}
