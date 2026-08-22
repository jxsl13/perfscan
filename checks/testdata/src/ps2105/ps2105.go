package ps2105

func countVowels(s string) int {
	n := 0
	for _, r := range []rune(s) { // want `ranging over \[\]rune\(s\) allocates a rune slice per loop entry; range s directly — a string range yields the same runes`
		switch r {
		case 'a', 'e', 'i', 'o', 'u':
			n++
		}
	}
	return n
}

func runeCount(s string) int {
	n := 0
	for range []rune(s) { // want `ranging over \[\]rune\(s\) allocates a rune slice per loop entry; range s directly — a string range yields the same runes`
		n++
	}
	return n
}

type config struct{ Name string }

func fromSelector(cfg config) rune {
	var last rune
	for _, r := range []rune(cfg.Name) { // want `ranging over \[\]rune\(cfg\.Name\) allocates a rune slice per loop entry; range cfg\.Name directly — a string range yields the same runes`
		last = r
	}
	return last
}

// GUARD: the key of a direct string range is the BYTE OFFSET, whereas
// over []rune(s) it is the rune index — a used key must stay unfixed.
func runeIndexOf(s string, want rune) int {
	for i, r := range []rune(s) { // want `ranging over \[\]rune\(s\) allocates a rune slice per loop entry; a direct range over s yields the same runes, but its key is the byte offset, not the rune index — the key is used here, so the rewrite is manual`
		if r == want {
			return i
		}
	}
	return -1
}

// GUARD: key-only form uses the rune index too — advisory, no fix.
func lastRuneIndex(s string) int {
	last := -1
	for i := range []rune(s) { // want `ranging over \[\]rune\(s\) allocates a rune slice per loop entry; a direct range over s yields the same runes, but its key is the byte offset, not the rune index — the key is used here, so the rewrite is manual`
		last = i
	}
	return last
}

// Blank key but the operand embeds a composite literal: the rewrite is withheld
// CONSERVATIVELY, and the message must say WHY accurately — not the misleading
// "the key is used here" (the key is blank). Advisory, no fix.
func compositeLitOperand() int {
	n := 0
	for _, r := range []rune(string([]byte{72, 73})) { // want `ranging over \[\]rune\(string\(\[\]byte\{72, 73\}\)\) allocates a rune slice per loop entry; a direct range over string\(\[\]byte\{72, 73\}\) yields the same runes, but string\(\[\]byte\{72, 73\}\) embeds a composite literal, so the rewrite is left manual`
		n += int(r)
	}
	return n
}

// Silent: already ranging the string directly.
func direct(s string) int {
	n := 0
	for _, r := range s {
		n += int(r)
	}
	return n
}

// Silent: ranging a real rune slice — no conversion, nothing to drop.
func realSlice(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += int(r)
	}
	return n
}

// Silent: []byte(s) iterates BYTES, not runes — a different pattern.
func byteConv(s string) int {
	n := 0
	for _, b := range []byte(s) {
		n += int(b)
	}
	return n
}
