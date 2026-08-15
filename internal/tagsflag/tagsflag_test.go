package tagsflag_test

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/go-extras/qtlint/internal/tagsflag"
)

func TestSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  []string
	}{{
		name:  "empty value carries no tags",
		value: "",
		want:  nil,
	}, {
		name:  "single tag",
		value: "integration",
		want:  []string{"integration"},
	}, {
		name:  "commas separate tags",
		value: "integration,e2e",
		want:  []string{"integration", "e2e"},
	}, {
		name:  "empty entries are dropped",
		value: "integration,,e2e,",
		want:  []string{"integration", "e2e"},
	}, {
		// The spelling go keeps for compatibility with Go 1.12 and earlier.
		name:  "spaces separate tags",
		value: "integration e2e",
		want:  []string{"integration", "e2e"},
	}, {
		name:  "runs of space separate tags once",
		value: "  integration   e2e  ",
		want:  []string{"integration", "e2e"},
	}, {
		// A value holding a space takes go's space-splitting path whole, so a
		// comma inside it stays part of the token and matches no constraint.
		// Splitting on both would satisfy a tag go leaves unsatisfied.
		name:  "a space makes the whole value space separated",
		value: "integration, e2e",
		want:  []string{"integration,", "e2e"},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tagsflag.Split(tc.value); !slices.Equal(got, tc.want) {
				t.Errorf("Split(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestForward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		goflags string
		want    string
		wantOK  bool
	}{{
		name:   "no tags flag leaves GOFLAGS alone",
		args:   []string{"-fix", "./..."},
		wantOK: false,
	}, {
		name:    "no tags flag does not disturb an inherited GOFLAGS",
		args:    []string{"./..."},
		goflags: "-tags=integration -mod=mod",
		wantOK:  false,
	}, {
		name:   "value in the next argument",
		args:   []string{"-tags", "integration", "./..."},
		want:   "-tags=integration",
		wantOK: true,
	}, {
		name:   "value joined with equals",
		args:   []string{"-tags=integration", "./..."},
		want:   "-tags=integration",
		wantOK: true,
	}, {
		name:   "two leading dashes",
		args:   []string{"--tags", "integration", "./..."},
		want:   "-tags=integration",
		wantOK: true,
	}, {
		name:   "comma separated value passes through",
		args:   []string{"-tags", "integration,e2e", "./..."},
		want:   "-tags=integration,e2e",
		wantOK: true,
	}, {
		// GOFLAGS is split on spaces, so the space spelling has to be
		// re-encoded with commas to survive the trip.
		name:   "space separated value is re-encoded with commas",
		args:   []string{"-tags", "integration e2e", "./..."},
		want:   "-tags=integration,e2e",
		wantOK: true,
	}, {
		// go's -tags replaces on repeat rather than accumulating.
		name:   "a repeated flag takes the last value",
		args:   []string{"-tags", "integration", "-tags=e2e", "./..."},
		want:   "-tags=e2e",
		wantOK: true,
	}, {
		name:    "an inherited GOFLAGS is kept and overridden",
		args:    []string{"-tags", "integration", "./..."},
		goflags: "-mod=mod -tags=stale",
		want:    "-mod=mod -tags=stale -tags=integration",
		wantOK:  true,
	}, {
		// A token that cannot reach GOFLAGS could not have matched a build
		// constraint either, so dropping it lands on go's own outcome.
		name:   "a token that cannot be a build tag is dropped",
		args:   []string{"-tags", "integration, e2e", "./..."},
		want:   "-tags=e2e",
		wantOK: true,
	}, {
		name:   "a quoted value cannot name a tag and forwards none",
		args:   []string{"-tags", "'integration e2e'", "./..."},
		want:   "-tags=",
		wantOK: true,
	}, {
		name:   "an explicitly empty value clears the tags",
		args:   []string{"-tags=", "./..."},
		want:   "-tags=",
		wantOK: true,
	}, {
		name:   "a trailing tags flag has no value to forward",
		args:   []string{"./...", "-tags"},
		wantOK: false,
	}, {
		name:   "a flag terminator ends the scan",
		args:   []string{"--", "-tags", "integration"},
		wantOK: false,
	}, {
		name:   "a lone dash is not a flag",
		args:   []string{"-", "-tags=integration"},
		want:   "-tags=integration",
		wantOK: true,
	}, {
		name:   "a differently named flag is not tags",
		args:   []string{"-tagsx=integration", "-xtags=e2e", "./..."},
		wantOK: false,
	}, {
		// The scan cannot know how many arguments each driver flag takes, so
		// it keeps looking past arguments the flag package would have treated
		// as the end of the flags. Setting GOFLAGS for a command line the
		// driver will reject anyway costs nothing.
		name:   "the scan continues past a non-flag argument",
		args:   []string{"./...", "-tags", "integration"},
		want:   "-tags=integration",
		wantOK: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tagsflag.Forward(tc.args, tc.goflags)
			if ok != tc.wantOK {
				t.Fatalf("Forward(%q, %q) ok = %v, want %v", tc.args, tc.goflags, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("Forward(%q, %q) = %q, want %q", tc.args, tc.goflags, got, tc.want)
			}
			if !ok && got != "" {
				t.Errorf("Forward(%q, %q) = %q, want %q when not forwarding", tc.args, tc.goflags, got, "")
			}
		})
	}
}

// driverTagsUsage is the chunk the flag package writes for the driver's -tags
// shim: the whole entry arrives in one Write.
const driverTagsUsage = "  -tags string\n    \tno effect (deprecated)\n"

func TestUsageWriter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		chunk string
		want  string
	}{{
		name:  "the driver's inert description is corrected",
		chunk: driverTagsUsage,
		want:  "  -tags string\n    \ta comma-separated list of build tags to consider satisfied, as with go build -tags\n",
	}, {
		// Other shims are the driver's business and keep their wording.
		name:  "another deprecated shim is untouched",
		chunk: "  -source\n    \tno effect (deprecated)\n",
		want:  "  -source\n    \tno effect (deprecated)\n",
	}, {
		name:  "an unrelated flag entry is untouched",
		chunk: "  -fix\n    \tapply all suggested fixes\n",
		want:  "  -fix\n    \tapply all suggested fixes\n",
	}, {
		// Only the driver's inert wording is replaced, so a release of x/tools
		// that made -tags real keeps its own description.
		name:  "a -tags entry that does not claim to be inert is untouched",
		chunk: "  -tags string\n    \tbuild tags to satisfy\n",
		want:  "  -tags string\n    \tbuild tags to satisfy\n",
	}, {
		name:  "a parse error message is untouched",
		chunk: "flag provided but not defined: -bogus\n",
		want:  "flag provided but not defined: -bogus\n",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got strings.Builder

			n, err := tagsflag.UsageWriter(&got).Write([]byte(tc.chunk))
			if err != nil {
				t.Fatalf("Write(%q): %v", tc.chunk, err)
			}
			if n != len(tc.chunk) {
				t.Errorf("Write(%q) = %d, want %d", tc.chunk, n, len(tc.chunk))
			}
			if got.String() != tc.want {
				t.Errorf("Write(%q) wrote %q, want %q", tc.chunk, got.String(), tc.want)
			}
		})
	}
}

func TestUsageWriterReportsWriteErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("no room")

	_, err := tagsflag.UsageWriter(failingWriter{err: want}).Write([]byte(driverTagsUsage))
	if !errors.Is(err, want) {
		t.Errorf("Write error = %v, want %v", err, want)
	}
}

type failingWriter struct {
	err error
}

func (f failingWriter) Write([]byte) (int, error) {
	return 0, f.err
}

// Interface check: UsageWriter's result is used where an io.Writer is wanted.
var _ = func() io.Writer { return tagsflag.UsageWriter(io.Discard) }
