//go:build qtprobe

package hidden

// Name is the only non-test declaration, and it is constrained too, so the
// whole package vanishes from ./... unless the tag is set.
const Name = "hidden"
