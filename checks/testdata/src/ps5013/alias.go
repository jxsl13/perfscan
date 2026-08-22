package ps5013

import buf "bytes"

// An aliased bytes import keeps its qualifier verbatim; only the selected
// name and the needle scaffolding are rewritten — no import surgery.
func aliased(b []byte) int {
	return buf.LastIndex(b, []byte{'@'}) // want `bytes\.LastIndex of the one-byte needle \[\]byte\{'@'\} builds a throwaway slice`
}
