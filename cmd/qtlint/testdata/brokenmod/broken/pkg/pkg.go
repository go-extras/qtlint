package pkg

// Broken refers to something that does not exist, so this package cannot be
// analyzed. Being unanalyzable is the whole point of the fixture.
func Broken() int { return undefinedThing }
