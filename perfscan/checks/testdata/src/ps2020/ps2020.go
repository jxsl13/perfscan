package ps2020

import (
	"bytes"
	"strings"
)

// The classic shape: rebuild b with a new separator via a throwaway
// [][]byte.
func rebuild(b []byte) []byte {
	out := bytes.Join(bytes.Split(b, []byte(",")), []byte("; ")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
	return out
}

// Directly returned.
func direct(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte("/")), []byte("-")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// An EMPTY replacement is deletion — the identity holds for every new
// (nil included), only the separator is gated.
func deletion(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte("\t")), nil) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// A []byte{...} composite literal with at least one element is statically
// non-empty even when the element VALUES are variables.
func compositeSep(b []byte, c byte) []byte {
	return bytes.Join(bytes.Split(b, []byte{c}), []byte{';'}) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// A keyed composite element still forces len > 0.
func keyedSep(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte{1: ','}), []byte(";")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// A NAMED constant inside the conversion is still a compile-time
// non-empty string; the replacement may be a variable — it is evaluated
// once either way.
const fieldSep = "|"

func namedSep(b, repl []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte(fieldSep)), repl) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// Multibyte constant separator: Index is byte-oriented and both forms
// match it identically.
func multibyte(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte("é")), []byte("e")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// Parenthesized inner call: still directly the Join argument.
func parenthesized(b []byte) []byte {
	return bytes.Join((bytes.Split(b, []byte(","))), []byte(" ")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// A comment between b and the separator sits inside an untouched span
// and survives the rewrite.
func commented(b []byte) []byte {
	return bytes.Join(bytes.Split(b /* input */, []byte(",")), []byte(";")) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// A side-effecting replacement expression is safe: it is evaluated
// exactly once in both forms, and Split/ReplaceAll are pure, so moving
// the call boundary is unobservable.
func sideEffect(b []byte, f func() []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte(",")), f()) // want `bytes\.Join\(bytes\.Split\(b, sep\), new\) with a statically non-empty separator allocates a throwaway \[\]\[\]byte of every fragment; bytes\.ReplaceAll\(b, sep, new\) is bit-identical in one scan`
}

// --- guards: none of the following may be reported or rewritten ---

// The EMPTY separator is the one divergence: Split(b, empty) explodes b
// after each UTF-8 sequence and Join fills the k-1 gaps, while
// ReplaceAll(b, empty, new) inserts new k+1 times.
func emptySepConv(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte("")), []byte(";"))
}

// An empty composite literal is the same divergence.
func emptySepLit(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte{}), []byte(";"))
}

// A nil separator behaves like the empty separator — same divergence.
func nilSep(b []byte) []byte {
	return bytes.Join(bytes.Split(b, nil), []byte(";"))
}

// A named constant that is empty is the same divergence.
const emptyConst = ""

func emptyNamed(b []byte) []byte {
	return bytes.Join(bytes.Split(b, []byte(emptyConst)), []byte(";"))
}

// A VARIABLE separator may be empty or nil at run time — never reported,
// because the suggestion would be wrong for those values.
func varSep(b, sep []byte) []byte {
	return bytes.Join(bytes.Split(b, sep), []byte(";"))
}

// A conversion of a VARIABLE string may be "" at run time too.
func varConvSep(b []byte, sep string) []byte {
	return bytes.Join(bytes.Split(b, []byte(sep)), []byte(";"))
}

// The slice stored first may have other consumers — out of scope.
func storedFirst(b []byte) []byte {
	parts := bytes.Split(b, []byte(","))
	return bytes.Join(parts, []byte(";"))
}

// SplitN limits the pieces: the tail keeps its separators and the
// identity does not hold.
func splitN(b []byte) []byte {
	return bytes.Join(bytes.SplitN(b, []byte(","), 3), []byte(";"))
}

// SplitAfter keeps the separator attached to each piece — Join over it
// is not a replacement.
func splitAfter(b []byte) []byte {
	return bytes.Join(bytes.SplitAfter(b, []byte(",")), []byte(";"))
}

// Fields splits on runs of whitespace and drops them — not a
// replacement either.
func fields(b []byte) []byte {
	return bytes.Join(bytes.Fields(b), []byte(" "))
}

// Joining any other slice is out of scope.
func otherSlice(parts [][]byte) []byte {
	return bytes.Join(parts, []byte(","))
}

// The strings twin is out of scope for this check (PS2015 owns it).
func stringsTwin(s string) string {
	return strings.Join(strings.Split(s, ","), ";")
}

// A shadowed bytes is not the bytes package.
type fakeBytes struct{}

func (fakeBytes) Split(b, sep []byte) [][]byte       { return [][]byte{b} }
func (fakeBytes) Join(p [][]byte, sep []byte) []byte { return sep }

func shadowedPkg(b []byte) []byte {
	bytes := fakeBytes{}
	return bytes.Join(bytes.Split(b, []byte(",")), []byte(";"))
}
