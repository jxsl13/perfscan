package ps5060

import (
	"fmt"
	"strconv"
)

var _ = fmt.Println
var _ = strconv.Atoi

type buffer []byte

type myBool bool

func sideEffect() bool { return true }

// --- POSITIVES ---

func boolBoth(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "ok=%t;", b) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

func quotePrefix(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "s=%q", s) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

func boolSuffix(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "%t!", b) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

// Trailing-only literal: operand evaluated first, no inert guard needed.
func suffixSideEffect(buf []byte) []byte {
	return fmt.Appendf(buf, "%t.", sideEffect()) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

// --- ADVISORY: reported, no fix ---

func namedDst(dst buffer, b bool) buffer {
	return fmt.Appendf(dst, "ok=%t;", b) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

func leadingSideEffect(buf []byte) []byte {
	return fmt.Appendf(buf, "b=%t", sideEffect()) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

func commentInside(buf []byte, b bool) []byte {
	return fmt.Appendf(buf, "ok=%t;" /* keep */, b) // want `fmt\.Appendf splicing one %t or %q verb into literal text`
}

// --- NEGATIVES: silent ---

// Bare verb with no literal text is PS5041's (for %q).
func bareVerb(buf []byte, s string) []byte {
	return fmt.Appendf(buf, "%q", s)
}

// A named bool type: its Format method could hijack %t.
func namedBool(buf []byte, m myBool) []byte {
	return fmt.Appendf(buf, "v=%t", m)
}

// %q over a rune is a different encoding (AppendQuoteRune), not this pattern.
func quoteRune(buf []byte, r rune) []byte {
	return fmt.Appendf(buf, "c=%q", r)
}

// %d is PS5059's.
func intVerb(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "n=%d", n)
}

// A literal nil destination is unspellable as an append chain.
func nilDest(b bool) []byte {
	return fmt.Appendf(nil, "ok=%t;", b)
}
