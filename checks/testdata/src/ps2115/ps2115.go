package ps2115

// --- reported: whole-string decode + allocation to read one rune ---

func first(s string) rune {
	return []rune(s)[0] // want `\[\]rune\(s\)\[0\] decodes and allocates every rune of the string to read the first; utf8\.DecodeRuneInString\(s\) decodes only that rune — note it returns \(rune, size\) and yields \(U\+FFFD, 0\) instead of panicking on an empty string`
}

func nth(s string, i int) rune {
	return []rune(s)[i] // want `\[\]rune\(s\)\[i\] decodes and allocates every rune of the string to read one; decode forward with utf8\.DecodeRuneInString or a counted for-range over s to stop at that rune`
}

func third(s string) rune {
	return []rune(s)[2] // want `\[\]rune\(s\)\[2\] decodes and allocates every rune of the string to read one; decode forward with utf8\.DecodeRuneInString or a counted for-range over s to stop at that rune`
}

// A named constant index of 0 still gets the first-rune remedy.
const zero = 0

func namedZeroIndex(s string) rune {
	return []rune(s)[zero] // want `\[\]rune\(s\)\[zero\] decodes and allocates every rune of the string to read the first; utf8\.DecodeRuneInString\(s\) decodes only that rune — note it returns \(rune, size\) and yields \(U\+FFFD, 0\) instead of panicking on an empty string`
}

// Parentheses around the conversion do not hide the pattern.
func parenthesized(s string) rune {
	return ([]rune(s))[1] // want `\[\]rune\(s\)\[1\] decodes and allocates every rune of the string to read one; decode forward with utf8\.DecodeRuneInString or a counted for-range over s to stop at that rune`
}

// Named types on either side pay the identical conversion.
type myString string

type runeSlice []rune

func namedTypes(m myString) rune {
	return runeSlice(m)[0] // want `runeSlice\(m\)\[0\] decodes and allocates every rune of the string to read the first; utf8\.DecodeRuneInString\(m\) decodes only that rune — note it returns \(rune, size\) and yields \(U\+FFFD, 0\) instead of panicking on an empty string`
}

// Used inside a larger expression: still one throwaway slice per read.
func compared(s string) bool {
	return []rune(s)[0] == 'x' // want `\[\]rune\(s\)\[0\] decodes and allocates every rune of the string to read the first`
}

// --- guards: none of the following may be reported ---

// Indexing an existing rune slice converts nothing.
func alreadySlice(rs []rune) rune {
	return rs[0]
}

// []byte(s)[0] is a byte read, not a rune decode — different pattern.
func byteConv(s string) byte {
	return []byte(s)[0]
}

// Direct string indexing reads a byte with no conversion at all.
func stringIndex(s string) byte {
	return s[0]
}

// The whole slice is kept: the conversion is genuinely needed.
func wholeUse(s string) []rune {
	rs := []rune(s)
	return rs
}

// Slicing keeps a run of runes, not one — out of this check's scope.
func sliced(s string) []rune {
	return []rune(s)[1:]
}

// Ranging over []rune(s) is PS2105's territory, not an index read.
func ranged(s string) rune {
	var last rune
	for _, r := range s {
		last = r
	}
	return last
}

// A write into the discarded slice reads no rune; the advice here does
// not apply.
func assignTarget(s string) {
	[]rune(s)[0] = 'x'
}

// Taking the element's address needs the slice to exist; no remedy here.
func addressTaken(s string) *rune {
	return &[]rune(s)[0]
}

// A function that merely looks like a conversion is not one.
func torunes(s string) []rune { return []rune(s) }

func fakeConv(s string) rune {
	return torunes(s)[0]
}
