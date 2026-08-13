package ps3101shadow

// Package-level shadow of the stdlib package name, declared in a DIFFERENT
// file than the use site: the parser leaves id.Obj nil for cross-file refs.
type fakeBytes struct{}

// Contains MUTATES its second argument — nothing like stdlib bytes.Contains.
func (fakeBytes) Contains(_, b []byte) bool {
	if len(b) > 0 {
		b[0] = 'X'
	}
	return false
}

var bytes fakeBytes
