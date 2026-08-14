package ps5013

import "bytes"

func basicComposite(b []byte) int {
	return bytes.Index(b, []byte{'/'}) // want `bytes\.Index of the one-byte needle \[\]byte\{'/'\} builds a throwaway slice and runs the generic substring search; bytes\.IndexByte\(b, '/'\) takes the same byte directly`
}

func basicConversion(b []byte) int {
	return bytes.Index(b, []byte("z")) // want `bytes\.Index of the one-byte needle \[\]byte\("z"\) builds a throwaway slice and runs the generic substring search; bytes\.IndexByte\(b, "z"\[0\]\) takes the same byte directly`
}

// LastIndex delegates to the backward byte scan for a one-byte needle;
// LastIndexByte IS that scan, with no dispatch and no needle slice.
func last(b []byte) int {
	return bytes.LastIndex(b, []byte{'/'}) // want `bytes\.LastIndex of the one-byte needle \[\]byte\{'/'\} builds a throwaway slice and runs the generic substring search; bytes\.LastIndexByte\(b, '/'\) takes the same byte directly`
}

func lastConversion(b []byte) int {
	return bytes.LastIndex(b, []byte("z")) // want `bytes\.LastIndex of the one-byte needle \[\]byte\("z"\) builds a throwaway slice and runs the generic substring search; bytes\.LastIndexByte\(b, "z"\[0\]\) takes the same byte directly`
}

func inControlFlow(b []byte) bool {
	if i := bytes.Index(b, []byte{':'}); i >= 0 { // want `bytes\.Index of the one-byte needle \[\]byte\{':'\} builds a throwaway slice`
		return true
	}
	return bytes.LastIndex(b, []byte(".")) > 0 // want `bytes\.LastIndex of the one-byte needle \[\]byte\("\."\) builds a throwaway slice`
}

// The composite element carries over VERBATIM: a named byte constant keeps
// its symbolic name, an integer spelling stays an integer spelling, an
// expression with side effects runs exactly once in the same argument
// position, and a variable element (not constant at all) is still
// statically one byte long by shape.
const slash = byte('/')

func next() byte { return 0 }

func compositeElements(b []byte, c byte) {
	_ = bytes.Index(b, []byte{slash})     // want `bytes\.IndexByte\(b, slash\) takes the same byte directly`
	_ = bytes.Index(b, []byte{0x2f})      // want `bytes\.IndexByte\(b, 0x2f\) takes the same byte directly`
	_ = bytes.Index(b, []byte{c})         // want `bytes\.IndexByte\(b, c\) takes the same byte directly`
	_ = bytes.LastIndex(b, []byte{b[0]})  // want `bytes\.LastIndexByte\(b, b\[0\]\) takes the same byte directly`
	_ = bytes.Index(b, []byte{next()})    // want `bytes\.IndexByte\(b, next\(\)\) takes the same byte directly`
	_ = bytes.Index(b, []uint8{'u'})      // want `bytes\.IndexByte\(b, 'u'\) takes the same byte directly`
	_ = bytes.LastIndex(b, []byte{'x',})  // want `bytes\.LastIndexByte\(b, 'x'\) takes the same byte directly`
}

// Escapes and raw literals in the conversion form carry over byte-verbatim:
// the ORIGINAL string literal token is reused and wrapped as lit[0], so no
// re-escaping ever happens. "\xff" is one raw byte (not valid UTF-8) and
// still in scope — a one-byte needle is matched byte-wise on both sides.
func escapes(b []byte) {
	_ = bytes.Index(b, []byte("\n"))       // want `bytes\.IndexByte\(b, "\\n"\[0\]\) takes the same byte directly`
	_ = bytes.Index(b, []byte("\xff"))     // want `bytes\.IndexByte\(b, "\\xff"\[0\]\) takes the same byte directly`
	_ = bytes.LastIndex(b, []byte("\\"))   // want `bytes\.LastIndexByte\(b, "\\\\"\[0\]\) takes the same byte directly`
	_ = bytes.LastIndex(b, []byte("\x00")) // want `bytes\.LastIndexByte\(b, "\\x00"\[0\]\) takes the same byte directly`
	_ = bytes.Index(b, []byte(`-`))        // want "bytes\\.IndexByte\\(b, `-`\\[0\\]\\) takes the same byte directly"
}

// Parentheses around and inside the needle are looked through; only the
// scaffolding around the kept token is deleted.
func parens(b []byte) int {
	return bytes.Index(b, ([]byte{';'})) + bytes.LastIndex(b, []byte(("!"))) // want `bytes\.IndexByte\(b, ';'\) takes the same byte directly` `bytes\.LastIndexByte\(b, "!"\[0\]\) takes the same byte directly`
}

// The haystack expression passes through verbatim, so a nested match
// inside it is rewritten independently — the edits never overlap.
func nested(b []byte) int {
	return bytes.Index(b[bytes.Index(b, []byte{':'})+1:], []byte(":")) // want `bytes\.IndexByte\(b, ':'\) takes the same byte directly` `bytes\.IndexByte\(b, ":"\[0\]\) takes the same byte directly`
}

// --- advisory: reported but never rewritten ---

// A conversion of a named string constant (or any constant expression that
// is not a literal) keeps its symbolic name: spelling out a copy of its
// value would discard it. Index the constant itself (colon[0]) by hand.
const colon = ":"

func namedConst(b []byte) int {
	return bytes.Index(b, []byte(colon)) // want `bytes\.Index of the one-byte needle \[\]byte\(colon\) builds a throwaway slice and runs the generic substring search; bytes\.IndexByte\(b, colon\[0\]\) takes the same byte directly — identical index for every input; the needle is a constant expression, not a literal — rewrite to bytes\.IndexByte by hand`
}

func constExpr(b []byte) int {
	return bytes.LastIndex(b, []byte(":"+"")) // want `the needle is a constant expression, not a literal — rewrite to bytes\.LastIndexByte by hand`
}

// A comment inside the slice scaffolding the fix would delete withholds
// the fix — the comment is never destroyed.
func commented(b []byte) int {
	return bytes.Index(b, []byte{ /* the field separator */ ':'}) // want `a comment inside the needle's slice syntax withholds the automatic fix — rewrite by hand`
}
