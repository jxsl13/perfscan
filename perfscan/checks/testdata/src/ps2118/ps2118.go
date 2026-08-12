package ps2118

import (
	"bytes"
	"io"
)

func direct(w io.Writer, b []byte) {
	io.WriteString(w, string(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// Both return (int, error): the results survive the rewrite.
func withResults(w io.Writer, b []byte) (int, error) {
	return io.WriteString(w, string(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// A named type with underlying []byte converts to string and stays
// assignable to Write's []byte parameter.
type payload []byte

func namedSlice(w io.Writer, p payload) {
	io.WriteString(w, string(p)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// A non-selectable first argument is parenthesized by the fix.
func addressed(buf bytes.Buffer, b []byte) {
	io.WriteString(&buf, string(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// Parens around the conversion are stripped with the rest of the call.
func parenthesized(w io.Writer, b []byte) {
	io.WriteString(w, (string(b))) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// An alias of string is the same conversion.
type str = string

func aliased(w io.Writer, b []byte) {
	io.WriteString(w, str(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// Reported but NOT fixed: a comment inside the rewritten call would be
// destroyed by the rewrite.
func commented(w io.Writer, b []byte) {
	io.WriteString(w, string( /* keep me */ b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}

// --- guards: none of the following may be reported or rewritten ---

// The second argument is already a string: nothing is allocated here.
func plainString(w io.Writer, s string) {
	io.WriteString(w, s)
}

// A []rune conversion re-encodes; it is not a byte-for-byte write.
func runeSlice(w io.Writer, rs []rune) {
	io.WriteString(w, string(rs))
}

// A single rune converts to its UTF-8 encoding, not to one byte.
func singleRune(w io.Writer, r rune) {
	io.WriteString(w, string(r))
}

// A slice of a NAMED byte type converts to string, but is not assignable
// to Write's []byte parameter — the rewrite would not compile.
type myByte byte

func namedByteElem(w io.Writer, nb []myByte) {
	io.WriteString(w, string(nb))
}

// A shadowed io is not the io package.
type fakeIO struct{}

func (fakeIO) WriteString(w io.Writer, s string) (int, error) { return w.Write([]byte(s)) }

func shadowed(w io.Writer, b []byte) {
	io := fakeIO{}
	io.WriteString(w, string(b))
}

// A local value named string makes string(b) a function call, not a
// conversion — it could do anything.
func stringShadow(w io.Writer, b []byte) {
	string := func(p []byte) str { return str(p) }
	io.WriteString(w, string(b))
}

// string(b) handed to anything other than io.WriteString is out of scope.
func take(s string) int { return len(s) }

func otherCallee(b []byte) int {
	return take(string(b))
}

// A Write that delegates through io.WriteString to its type's
// WriteString: rewriting io.WriteString(e, string(p)) to e.Write(p) here
// would make Write call ITSELF — unbounded recursion that still
// compiles. The delegation is the correct code; nothing is reported.
type selfE struct{ b []byte }

func (e *selfE) WriteString(s string) (int, error) {
	e.b = append(e.b, s...)
	return len(s), nil
}

func (e *selfE) Write(p []byte) (int, error) {
	return io.WriteString(e, string(p))
}

// Writing to a DIFFERENT writer inside Write is still reported: w is not
// the receiver.
func (e *selfE) copyTo(w io.Writer, p []byte) (int, error) {
	return io.WriteString(w, string(p)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}
