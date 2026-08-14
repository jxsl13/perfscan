package ps2136

import (
	"strconv"
	sc "strconv"
)

// Itoa has no direct Append twin: AppendInt with a value-preserving
// int64(...) wrap and the explicit base 10 (Itoa is FormatInt(int64(i), 10)).
func itoa(n int) []byte {
	return []byte(strconv.Itoa(n)) // want `\[\]byte\(strconv\.Itoa\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

// The whole argument expression is wrapped: int64(n + 1).
func itoaExpr(n int) []byte {
	return []byte(strconv.Itoa(n + 1)) // want `\[\]byte\(strconv\.Itoa\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

// Format* keeps every argument verbatim; only nil is prepended.
func formatInt(x int64, base int) []byte {
	return []byte(strconv.FormatInt(x, base)) // want `\[\]byte\(strconv\.FormatInt\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

func formatUint(u uint64) []byte {
	return []byte(strconv.FormatUint(u, 16)) // want `\[\]byte\(strconv\.FormatUint\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendUint\(nil, \.\.\.\) writes the bytes directly`
}

func formatFloat(f float64) []byte {
	return []byte(strconv.FormatFloat(f, 'g', -1, 64)) // want `\[\]byte\(strconv\.FormatFloat\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendFloat\(nil, \.\.\.\) writes the bytes directly`
}

func formatBool(b bool) []byte {
	return []byte(strconv.FormatBool(b)) // want `\[\]byte\(strconv\.FormatBool\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendBool\(nil, \.\.\.\) writes the bytes directly`
}

func quote(s string) []byte {
	return []byte(strconv.Quote(s)) // want `\[\]byte\(strconv\.Quote\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendQuote\(nil, \.\.\.\) writes the bytes directly`
}

func quoteToASCII(s string) []byte {
	return []byte(strconv.QuoteToASCII(s)) // want `\[\]byte\(strconv\.QuoteToASCII\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendQuoteToASCII\(nil, \.\.\.\) writes the bytes directly`
}

func quoteRune(r rune) []byte {
	return []byte(strconv.QuoteRune(r)) // want `\[\]byte\(strconv\.QuoteRune\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendQuoteRune\(nil, \.\.\.\) writes the bytes directly`
}

func quoteRuneToGraphic(r rune) []byte {
	return []byte(strconv.QuoteRuneToGraphic(r)) // want `\[\]byte\(strconv\.QuoteRuneToGraphic\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendQuoteRuneToGraphic\(nil, \.\.\.\) writes the bytes directly`
}

// A parenthesized inner call: the shell and its parens are both removed.
func parens(n int) []byte {
	return []byte((strconv.Itoa(n))) // want `\[\]byte\(strconv\.Itoa\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

// An aliased strconv import keeps its qualifier: only the selected name
// changes, so the rewrite compiles under the alias.
func aliased(n int) []byte {
	return []byte(sc.Itoa(n)) // want `\[\]byte\(strconv\.Itoa\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

// --- advisory: reported but NOT rewritten ---

// A comment inside the deleted conversion shell would be destroyed by the
// fix: the report stays advisory.
func commented(n int) []byte {
	return []byte( /* keep me */ strconv.Itoa(n)) // want `\[\]byte\(strconv\.Itoa\(\.\.\.\)\) allocates a string then copies it; strconv\.AppendInt\(nil, \.\.\.\) writes the bytes directly`
}

// --- guards: no report at all ---

type Bytes []byte

// A named byte-slice conversion changes the expression's type:
// strconv.Append* returns []byte, not Bytes, so the rewrite would not
// type-check. Out of scope.
func named(n int) Bytes {
	return Bytes(strconv.Itoa(n))
}

// []rune is not []byte.
func runes(n int) []rune {
	return []rune(strconv.Itoa(n))
}

// The intermediate string used on its own is not the pattern.
func keepString(n int) string {
	return strconv.Itoa(n)
}

// A plain string conversion has no strconv call inside.
func plain(s string) []byte {
	return []byte(s)
}

// An Itoa on a value named strconv is not the standard library.
type fake struct{}

func (fake) Itoa(n int) string { return "" }

func local(n int) []byte {
	var strconv fake
	return []byte(strconv.Itoa(n))
}
