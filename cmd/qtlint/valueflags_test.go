// Drift guard for the driver flag table in internal/modules.
//
// It lives beside the command because that is where the binary is built, and in
// its own file because it is the one multi-module test that reaches into the
// package rather than driving the command as a program.
package main_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-extras/qtlint/internal/modules"
)

// TestValueFlagsMatchesTheDriver pins the one piece of the driver's flag set
// this change has to know about.
//
// Deciding which arguments are package operands means knowing which flags take
// their value as a separate argument, and the driver builds its flag set inside
// singlechecker.Main — after the point where that decision has to be made. So
// the list is written down, and this reads the built binary's own -h to check
// it is still right. An x/tools release that adds a value-taking flag turns
// into a failure here rather than into a command line silently misread.
func TestValueFlagsMatchesTheDriver(t *testing.T) {
	t.Parallel()

	got := runMultimod(t, "-h")

	var takesValue []string

	for _, line := range strings.Split(got.output, "\n") {
		if !strings.HasPrefix(line, "  -") {
			continue
		}

		// The flag package writes "  -name" for a boolean and
		// "  -name type" for anything else, then a tab before any usage
		// text it puts on the same line.
		head, _, _ := strings.Cut(strings.TrimPrefix(line, "  -"), "\t")
		if name, _, ok := strings.Cut(strings.TrimSpace(head), " "); ok {
			takesValue = append(takesValue, name)
		}
	}

	slices.Sort(takesValue)

	if len(takesValue) == 0 {
		t.Fatalf("read no flags out of -h, so this proves nothing:\n%s", got.output)
	}

	want := modules.ValueFlags()
	if !slices.Equal(takesValue, want) {
		t.Errorf("the driver's value-taking flags have changed\n"+
			"driver -h: %q\nmodules:   %q\n"+
			"update valueFlags in internal/modules", takesValue, want)
	}
}
