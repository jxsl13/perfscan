package ps2124

import (
	"strings"
)

func sink(string) {}

const dash = "-"

// Assignment context: two elements, the "/" separator interleaved.
func twoElems(a, b string) string {
	s := strings.Join([]string{a, b}, "/") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
	return s
}

// Three elements with the EMPTY separator: plain concatenation.
func threeEmpty(a, b, c string) string {
	return strings.Join([]string{a, b, c}, "") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A single element: strings.Join is the identity; sep is never used.
func single(a string) string {
	return strings.Join([]string{a}, "/") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A single element with a NAMED-constant separator: sep is dropped, and
// a same-package constant leaves no import behind to orphan.
func singleNamedSep(a string) string {
	return strings.Join([]string{a}, dash) // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A single PRIMARY element needs no parentheses even under an index.
func singleIndexed(a string) byte {
	return strings.Join([]string{a}, "/")[0] // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A single + chain element under an index DOES need them.
func singleConcatIndexed(a, b string) byte {
	return strings.Join([]string{a + b}, "/")[0] // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// A named-constant separator: its source text is interleaved verbatim.
func namedSep(a, b string) string {
	return strings.Join([]string{a, b}, dash) // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// An element that is itself a strings call stays verbatim and keeps the
// import in use.
func callElem(x, y string) string {
	return strings.Join([]string{strings.ToUpper(x), y}, "/") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// An element that is itself a + concatenation needs no parentheses:
// string + is associative and the only string-producing binary operator.
func concatElem(a, b, c string) string {
	return strings.Join([]string{a + b, c}, "-") // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// Call-argument context is self-delimiting: no parentheses needed.
func asArg(a, b string) {
	sink(strings.Join([]string{a, b}, ",")) // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// PRECEDENCE: indexing binds tighter than + — the whole replacement must
// be parenthesized or the index would apply to the last operand only.
func indexed(a, b string) byte {
	return strings.Join([]string{a, b}, "-")[0] // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
}

// Reported but NOT fixed: a comment inside the rewritten scaffolding
// would be destroyed by the edits.
func commented(a, b string) string {
	return strings.Join([]string{ // keep me // want `strings\.Join over an inline \[\]string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved \+ concatenation builds the identical string`
		a, b}, "/")
}

// --- guards: none of the following may be reported or rewritten ---

// A VARIABLE slice is not an inline literal: other consumers may hold
// it and its length is unknown.
func varSlice(xs []string) string {
	return strings.Join(xs, ",")
}

// A NAMED string type cannot even be spelled into a []string literal
// (S is not assignable to string), and []S is not assignable to Join's
// []string parameter — the type system already excludes named-string
// elements; the in-check element guard is defense in depth.

// A NON-CONSTANT separator would have its evaluation multiplied by the
// interleaving: never matched.
func varSep(a, b, sep string) string {
	return strings.Join([]string{a, b}, sep)
}

// Keyed elements: slice positions need not match source order.
func keyed(a, b string) string {
	return strings.Join([]string{1: b, 0: a}, ",")
}

// Zero elements: Join returns "" — out of scope.
func empty() string {
	return strings.Join([]string{}, ",")
}

// A shadowed strings is not the strings package.
type fakeStrings struct{}

func (fakeStrings) Join(elems []string, sep string) string { return "" }

func shadowed(a, b string) string {
	strings := fakeStrings{}
	return strings.Join([]string{a, b}, ",")
}
