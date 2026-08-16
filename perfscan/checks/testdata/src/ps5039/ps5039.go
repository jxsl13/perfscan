package ps5039

type buffer []byte

type myRune rune

type namedString string

func next() rune { return 'x' }

// --- fixable: dst is the unnamed []byte, the operand fits int32 ---

// The plain rune form: the conversion scaffolding vanishes, the operand
// stays verbatim. The first fixable site carries the unicode/utf8
// import edit (once per file).
func runeVar(dst []byte, r rune) []byte {
	return append(dst, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// A second fixable site in the same file: no second import edit.
func runeVarAgain(dst []byte, r rune) []byte {
	dst = append(dst, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
	return dst
}

// int32 IS rune: the operand is assignable and kept verbatim.
func int32Var(dst []byte, i int32) []byte {
	return append(dst, string(i)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// A named rune type is int32-lossless but not assignable to rune — the
// fix wraps it as the value-preserving rune(m).
func namedRune(dst []byte, m myRune) []byte {
	return append(dst, string(m)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// The narrower integer widths widen value-preservingly as rune(x).
func byteVar(dst []byte, c byte) []byte {
	return append(dst, string(c)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

func int8Var(dst []byte, v int8) []byte {
	return append(dst, string(v)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

func int16Var(dst []byte, v int16) []byte {
	return append(dst, string(v)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

func uint16Var(dst []byte, v uint16) []byte {
	return append(dst, string(v)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// A parenthesized conversion still matches; the redundant parentheses
// vanish with the scaffolding.
func parens(dst []byte, r rune) []byte {
	return append(dst, (string(r))...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// A side-effecting operand passes through verbatim: evaluated exactly
// once in both spellings, in the same order.
func sideEffect(dst []byte) []byte {
	return append(dst, string(next())...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// dst may be any []byte-typed expression; it is kept verbatim too.
func indexedDst(bufs [][]byte, i int, r rune) {
	bufs[i] = append(bufs[i], string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// --- advisory: dst has a NAMED byte-slice type ---

// append returns dst's own named type; utf8.AppendRune returns []byte —
// the rewrite would change the expression's static type, so the report
// stays advisory.
func namedDst(b buffer, r rune) buffer {
	return append(b, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst; dst's static type is not the plain \[\]byte and utf8\.AppendRune returns \[\]byte, so the rewrite would change the expression's static type — rewrite by hand if nothing relies on it`
}

// --- advisory: a comment inside the rewritten scaffolding ---

func commented(dst []byte, r rune) []byte {
	return append(dst /* keep */, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// --- advisory: a shadowed utf8 qualifier at the call site ---

func shadowed(dst []byte, r rune) []byte {
	utf8 := 1
	_ = utf8
	return append(dst, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

// --- silent negatives ---

// A CONSTANT rune conversion is folded into a constant string at
// compile time and allocates nothing — no report.
func constantRune(dst []byte) []byte {
	return append(dst, string('a')...)
}

const kr rune = 'x'

func constantNamed(dst []byte) []byte {
	return append(dst, string(kr)...)
}

// WIDER integer types are never touched: string(x) converts the FULL
// value (out of range becomes U+FFFD) while rune(x) would truncate
// first and diverge.
func wider(dst []byte, i int, i64 int64, u32 uint32, up uintptr) []byte {
	dst = append(dst, string(i)...)
	dst = append(dst, string(i64)...)
	dst = append(dst, string(u32)...)
	dst = append(dst, string(up)...)
	return dst
}

// A NAMED string type's conversion is not the predeclared string
// conversion — silent.
func namedStringConv(dst []byte, r rune) []byte {
	return append(dst, namedString(r)...)
}

// A plain string spread has no conversion to remove — silent.
func plainString(dst []byte, s string) []byte {
	return append(dst, s...)
}

// string of a []rune encodes the WHOLE slice, not a single rune — silent.
func runeSlice(dst []byte, rs []rune) []byte {
	return append(dst, string(rs)...)
}

// A []byte conversion spread is a different pattern — silent here.
func byteSliceConv(dst []byte, s string) []byte {
	return append(dst, []byte(s)...)
}

// Not a spread: appending a single byte element — silent.
func nonSpread(dst []byte, r rune) []byte {
	return append(dst, byte(r))
}

// More than two arguments — silent.
func threeArgs(dst []byte, a, b byte) []byte {
	return append(dst, a, b)
}
