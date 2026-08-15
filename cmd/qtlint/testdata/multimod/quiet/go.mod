// A second nested module whose only violation sits behind a build constraint.
// Without the tag it is not an error, it is simply absent, so a run that never
// reached this module and a run that reached it and found nothing look the
// same from the outside. That is the shape this fixture exists to tell apart.
module qtlint.test/quiet

go 1.21

require github.com/frankban/quicktest v0.0.0

replace github.com/frankban/quicktest => ../quicktest
