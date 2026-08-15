package ps2026

import bb "bytes"

// An aliased import keeps its qualifier verbatim: the constructor is
// still recognized by type info, and only the len(...) scaffolding goes.
func aliased(s string) int {
	return len(bb.NewBufferString(s).String()) // want `bb\.NewBufferString\(s\)\.Len\(\) returns the identical int in O\(1\) with zero allocation`
}

func aliasedValue() int {
	var buf bb.Buffer
	buf.WriteByte('x')
	return len(buf.String()) // want `buf\.Len\(\) returns the identical int in O\(1\) with zero allocation`
}
