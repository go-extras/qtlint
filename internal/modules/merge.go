package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// tree is the JSON document the driver prints under -json: a package
// identifier, then an analyzer name, then either the list of diagnostics that
// analyzer produced or an object describing why it produced none.
//
// Keeping the innermost value raw is what lets diagnostics travel from a child
// to the combined document without this package needing to know their fields.
// A future release of x/tools may add one; it will pass through untouched.
type tree map[string]map[string]json.RawMessage

// add merges one child's document into t.
//
// Two modules producing the same package identifier is not expected — an
// identifier carries the module path, and two modules with the same path in one
// repository is a mistake in the repository — but it is possible, and silently
// dropping one of them would lose diagnostics. Diagnostic lists are therefore
// concatenated, and an error beats a list, because an error means those
// packages were never analyzed and that is the more important fact about them.
func (t tree) add(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	var incoming tree
	if err := json.Unmarshal(data, &incoming); err != nil {
		// Refusing here is deliberate. Unparsable output means a module's
		// result is unknown, and an unknown result that is quietly skipped
		// leaves a combined document that looks complete.
		return fmt.Errorf("parse -json output: %w", err)
	}

	for pkg, analyzers := range incoming {
		if _, ok := t[pkg]; !ok {
			t[pkg] = analyzers

			continue
		}

		for name, value := range analyzers {
			existing, ok := t[pkg][name]
			if !ok {
				t[pkg][name] = value

				continue
			}

			combined, err := combine(existing, value)
			if err != nil {
				return err
			}

			t[pkg][name] = combined
		}
	}

	return nil
}

// combine joins two values recorded for the same package and analyzer.
func combine(a, b json.RawMessage) (json.RawMessage, error) {
	if !isArray(a) {
		return a, nil
	}

	if !isArray(b) {
		return b, nil
	}

	var first, second []json.RawMessage
	if err := json.Unmarshal(a, &first); err != nil {
		return nil, fmt.Errorf("parse diagnostics: %w", err)
	}

	if err := json.Unmarshal(b, &second); err != nil {
		return nil, fmt.Errorf("parse diagnostics: %w", err)
	}

	joined, err := json.Marshal(append(first, second...))
	if err != nil {
		return nil, fmt.Errorf("join diagnostics: %w", err)
	}

	return joined, nil
}

// isArray reports whether raw holds a JSON array, which is how a list of
// diagnostics is told apart from the object that reports an error.
func isArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)

	return len(trimmed) > 0 && trimmed[0] == '['
}

// print writes the combined document the way the driver writes its own:
// indented with tabs, keys in the order encoding/json sorts a map into, and a
// trailing newline. A caller parsing qtlint's -json output cannot tell that more
// than one process produced it.
func (t tree) print(w io.Writer) error {
	data, err := json.MarshalIndent(t, "", "\t")
	if err != nil {
		return fmt.Errorf("encode -json output: %w", err)
	}

	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		return fmt.Errorf("write -json output: %w", err)
	}

	return nil
}
