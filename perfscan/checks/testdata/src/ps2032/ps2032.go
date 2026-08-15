package ps2032

import (
	"strconv"
	sc "strconv"
)

// The shell of the predeclared string conversion is removed and the nil
// destination dropped; every remaining argument carries over verbatim.
func appendInt(x int64) string {
	return string(strconv.AppendInt(nil, x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// A non-constant base carries over untouched.
func appendIntBase(x int64, base int) string {
	return string(strconv.AppendInt(nil, x+1, base)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

func appendUint(u uint64) string {
	return string(strconv.AppendUint(nil, u, 16)) // want `string\(strconv\.AppendUint\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatUint\(\.\.\.\) formats the identical string directly`
}

// AppendFloat and FormatFloat share the (f, fmt, prec, bitSize)
// signature once the nil destination is dropped.
func appendFloat(f float64) string {
	return string(strconv.AppendFloat(nil, f, 'g', -1, 64)) // want `string\(strconv\.AppendFloat\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatFloat\(\.\.\.\) formats the identical string directly`
}

func appendBool(b bool) string {
	return string(strconv.AppendBool(nil, b)) // want `string\(strconv\.AppendBool\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatBool\(\.\.\.\) formats the identical string directly`
}

func quote(s string) string {
	return string(strconv.AppendQuote(nil, s)) // want `string\(strconv\.AppendQuote\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.Quote\(\.\.\.\) formats the identical string directly`
}

func quoteToASCII(s string) string {
	return string(strconv.AppendQuoteToASCII(nil, s)) // want `string\(strconv\.AppendQuoteToASCII\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.QuoteToASCII\(\.\.\.\) formats the identical string directly`
}

func quoteToGraphic(s string) string {
	return string(strconv.AppendQuoteToGraphic(nil, s)) // want `string\(strconv\.AppendQuoteToGraphic\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.QuoteToGraphic\(\.\.\.\) formats the identical string directly`
}

func quoteRune(r rune) string {
	return string(strconv.AppendQuoteRune(nil, r)) // want `string\(strconv\.AppendQuoteRune\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.QuoteRune\(\.\.\.\) formats the identical string directly`
}

func quoteRuneToASCII(r rune) string {
	return string(strconv.AppendQuoteRuneToASCII(nil, r)) // want `string\(strconv\.AppendQuoteRuneToASCII\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.QuoteRuneToASCII\(\.\.\.\) formats the identical string directly`
}

func quoteRuneToGraphic(r rune) string {
	return string(strconv.AppendQuoteRuneToGraphic(nil, r)) // want `string\(strconv\.AppendQuoteRuneToGraphic\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.QuoteRuneToGraphic\(\.\.\.\) formats the identical string directly`
}

// A parenthesized inner call: the shell and its parens are both removed.
func parens(x int64) string {
	return string((strconv.AppendInt(nil, x, 10))) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// A parenthesized nil is still the literal untyped nil; the parens go
// with it.
func parenNil(x int64) string {
	return string(strconv.AppendInt((nil), x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// An aliased strconv import keeps its qualifier: only the selected name
// changes, so the rewrite compiles under the alias.
func aliased(x int64) string {
	return string(sc.AppendInt(nil, x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// A NAMED string type keeps its conversion wrapper: T(string) is exactly
// as legal as T([]byte) and yields the same value.
type ID string

func named(u uint64) ID {
	return ID(strconv.AppendUint(nil, u, 10)) // want `string\(strconv\.AppendUint\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatUint\(\.\.\.\) formats the identical string directly`
}

// An alias OF the predeclared string is the identical type: the shell is
// removed like a plain string conversion.
type str = string

func aliasType(b bool) str {
	return str(strconv.AppendBool(nil, b)) // want `string\(strconv\.AppendBool\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatBool\(\.\.\.\) formats the identical string directly`
}

// Arguments spread over multiple lines carry over in place.
func multiline(x int64) string {
	return string(strconv.AppendInt( // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
		nil,
		x,
		2,
	))
}

// The pattern nested as another call's argument.
func nested(x int64) int {
	return len(string(strconv.AppendInt(nil, x, 10))) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// --- advisory: reported but NOT rewritten ---

// A comment inside the deleted conversion shell would be destroyed by
// the fix: the report stays advisory.
func commentedShell(x int64) string {
	return string( /* keep me */ strconv.AppendInt(nil, x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// A comment riding on the deleted nil argument likewise withholds the
// fix — in the named-wrapper form too.
func commentedNil(x int64) string {
	return string(strconv.AppendInt(nil /* dst */, x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

func commentedNamed(x int64) ID {
	return ID(strconv.AppendInt(nil /* c */, x, 10)) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

func commentedTail(x int64) string {
	return string(strconv.AppendInt(nil, x, 10) /* tail */) // want `string\(strconv\.AppendInt\(nil, \.\.\.\)\) formats into a throwaway \[\]byte and copies it into a string; strconv\.FormatInt\(\.\.\.\) formats the identical string directly`
}

// --- guards: no report at all ---

// A non-nil destination prepends that buffer's bytes: not the pattern.
func realBuffer(buf []byte, x int64) string {
	return string(strconv.AppendInt(buf, x, 10))
}

// []byte(nil) is provably empty but is not the literal nil: out of scope.
func typedNil(x int64) string {
	return string(strconv.AppendInt([]byte(nil), x, 10))
}

// An identifier NAMED nil that shadows the predeclared one can carry
// bytes that Append* would prepend.
func shadowedNil(x int64) string {
	nil := []byte("seed")
	return string(strconv.AppendInt(nil, x, 10))
}

// The []byte result used directly is not the pattern (that direction is
// PS2136's).
func keepBytes(x int64) []byte {
	return strconv.AppendInt(nil, x, 10)
}

// string() of a Format* call is a string-to-string conversion, not the
// pattern.
func ofFormat(x int64) string {
	return string(strconv.FormatInt(x, 10))
}

// A conversion to a type parameter is out of scope even when every
// instantiation has underlying string.
func generic[T ~string](x int64) T {
	return T(strconv.AppendInt(nil, x, 10))
}

// An AppendInt on a value named strconv is not the standard library.
type fake struct{}

func (fake) AppendInt(dst []byte, x int64, base int) []byte { return dst }

func local(x int64) string {
	var strconv fake
	return string(strconv.AppendInt(nil, x, 10))
}

// A shadowed string identifier makes the outer expression a plain call,
// not a conversion.
func shadowedString(x int64) []byte {
	string := func(b []byte) []byte { return b }
	return string(strconv.AppendInt(nil, x, 10))
}
