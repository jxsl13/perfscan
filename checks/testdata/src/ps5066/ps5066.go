package ps5066

import (
	"bytes"
	"strings"
)

// A wrapper type embedding bytes.Buffer could override Bytes().
type wrapper struct {
	bytes.Buffer
}

func (wrapper) Bytes() []byte { return nil }

// --- POSITIVES ---

func valueBuf(buf bytes.Buffer) byte {
	return buf.String()[0] // want `indexing bytes\.Buffer\.String\(\) copies the whole buffer to read one byte`
}

func ptrBuf(buf *bytes.Buffer, i int) byte {
	return buf.String()[i] // want `indexing bytes\.Buffer\.String\(\) copies the whole buffer to read one byte`
}

func exprIndex(buf *bytes.Buffer) byte {
	return buf.String()[buf.Len()-1] // want `indexing bytes\.Buffer\.String\(\) copies the whole buffer to read one byte`
}

// A comment before the method carries over (only String -> Bytes is renamed).
func leadingComment(buf *bytes.Buffer) byte {
	return buf. /*keep*/ String()[0] // want `indexing bytes\.Buffer\.String\(\) copies the whole buffer to read one byte`
}

// --- NEGATIVES: silent ---

// A slice, not an index: the result type would change from string to []byte.
func sliceNotIndex(buf *bytes.Buffer) string {
	return buf.String()[1:3]
}

// strings.Builder.String() is already zero-copy.
func builder(sb *strings.Builder) byte {
	return sb.String()[0]
}

// A wrapper type whose Bytes() is overridden.
func wrapped(w *wrapper) byte {
	return w.String()[0]
}

// Not indexed.
func whole(buf *bytes.Buffer) string {
	return buf.String()
}
