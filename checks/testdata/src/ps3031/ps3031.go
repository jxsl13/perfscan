package ps3031

import (
	"bytes"
	"unicode"
)

// The predicate arguments are this file's ONLY unicode references, so
// the fix also drops the orphaned unicode import (the runner never
// prunes imports itself). The bytes import stays: the rewrite keeps
// the qualifier and merely renames TrimFunc.
func basic(b []byte) []byte {
	return bytes.TrimFunc(b, unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

// The source argument is kept byte-verbatim, however it is spelled: a
// field selector, a compound expression with a call (evaluated exactly
// once in both forms), a named []byte type (assignable to TrimFunc's
// and TrimSpace's identical []byte parameter), and a parenthesized
// predicate argument.
type buf []byte

func verbatim(w struct{ b []byte }, f func() []byte, n buf) {
	_ = bytes.TrimFunc(w.b, unicode.IsSpace)              // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
	_ = bytes.TrimFunc(append(f(), '!'), unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
	_ = bytes.TrimFunc(n, unicode.IsSpace)                // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
	_ = bytes.TrimFunc(w.b, (unicode.IsSpace))            // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

// A nested match sits inside the kept source-argument span: both sites
// are rewritten (the edits never overlap).
func nested(b []byte) []byte {
	return bytes.TrimFunc(bytes.TrimFunc(b, unicode.IsSpace), unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes` `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}

// A multi-line call keeps the source argument verbatim; the deleted
// span swallows the predicate and the trailing comma up to the closing
// parenthesis.
func multiline(b []byte) []byte {
	return bytes.TrimFunc( // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
		append(b, b...),
		unicode.IsSpace,
	)
}

// A comment BEFORE the source argument sits in an untouched span and
// survives the rewrite.
func leadingComment(b []byte) []byte {
	return bytes.TrimFunc( /* keep me */ b, unicode.IsSpace) // want `bytes\.TrimFunc\(b, unicode\.IsSpace\) decodes`
}
