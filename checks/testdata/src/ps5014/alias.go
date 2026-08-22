package ps5014

import buf "bytes"

// An aliased bytes import keeps its qualifier verbatim; only the selected
// name, the needle scaffolding, and the appended comparison change — no
// import surgery.
func aliased(b []byte) bool {
	return buf.Contains(b, []byte{'@'}) // want `bytes\.Contains of the one-byte needle \[\]byte\{'@'\} builds a throwaway slice`
}
