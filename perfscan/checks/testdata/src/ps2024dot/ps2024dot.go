package ps2024dot

import . "unicode/utf8"

// Under a dot import the callee is a bare ident; the sibling resolves
// to unicode/utf8 here, so the fix applies with the bare name.
func dotFix(s string) int {
	return RuneCount([]byte(s)) // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}

func dotMirror(b []byte) int {
	return RuneCountInString(string(b)) // want `utf8\.RuneCountInString\(string\(b\)\) copies b into a throwaway string just to count its runes; utf8\.RuneCount\(b\) is the bit-identical, zero-copy count`
}

// The sibling name is shadowed at the call site — the bare rewrite
// would be captured, so the report stays advisory.
func dotShadow(s string) int {
	RuneCountInString := func(string) int { return 0 }
	_ = RuneCountInString
	return RuneCount([]byte(s)) // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}
