package ps2050

import "unicode/utf8"

type MyStr string

// --- POSITIVES: string(utf8.AppendRune(nil, r)), non-constant rune ---

func runeVar(r rune) string {
	return string(utf8.AppendRune(nil, r)) // want `string\(utf8\.AppendRune\(nil, r\)\) UTF-8-encodes r into a throwaway \[\]byte and copies it into a string; string\(r\) encodes the rune directly`
}

func int32Var(i int32) string {
	return string(utf8.AppendRune(nil, i)) // want `string\(utf8\.AppendRune\(nil, r\)\) UTF-8-encodes r into a throwaway \[\]byte and copies it into a string; string\(r\) encodes the rune directly`
}

// A NAMED string target keeps its conversion shell — MyStr(rune) is legal.
func namedTarget(r rune) MyStr {
	return MyStr(utf8.AppendRune(nil, r)) // want `string\(utf8\.AppendRune\(nil, r\)\) UTF-8-encodes r into a throwaway \[\]byte and copies it into a string; string\(r\) encodes the rune directly`
}

// --- ADVISORY: reported, no fix ---

func commentInside(r rune) string {
	return string(utf8.AppendRune(nil /* keep */, r)) // want `string\(utf8\.AppendRune\(nil, r\)\) UTF-8-encodes r into a throwaway \[\]byte and copies it into a string; string\(r\) encodes the rune directly`
}

// --- NEGATIVES: silent ---

// A non-nil buffer would prepend its bytes.
func nonNilBuf(buf []byte, r rune) string {
	return string(utf8.AppendRune(buf, r))
}

// A constant rune: string(<const>) could be a flagged int conversion.
func constRune() string {
	return string(utf8.AppendRune(nil, 'A'))
}

// A []byte conversion target, not a string.
func bytesTarget(r rune) []byte {
	return []byte(utf8.AppendRune(nil, r))
}
