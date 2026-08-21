package ps3025

import (
	"fmt"
)

// Keeps fmt referenced so the rewrites below do not orphan the import
// (orphan.go exercises the orphan path).
func keep() { fmt.Println("x") }

// --- POSITIVES: verbless string literal, unnamed []byte dest — fixed ---

func lit(buf []byte) []byte {
	return fmt.Appendf(buf, "literal text") // want `fmt\.Appendf on a verbless constant format`
}

func assigned(buf []byte) []byte {
	buf = fmt.Appendf(buf, "HTTP/1.1 200 OK\r\n") // want `fmt\.Appendf on a verbless constant format`
	return buf
}

// Escapes (NUL, invalid UTF-8) ride through both spellings verbatim.
func binary(buf []byte) []byte {
	return fmt.Appendf(buf, "\x00\xff\xc0\x80") // want `fmt\.Appendf on a verbless constant format`
}

// A raw literal stays byte-verbatim in place (its \n is two source bytes).
func raw(buf []byte) []byte {
	return fmt.Appendf(buf, `raw \n literal`) // want `fmt\.Appendf on a verbless constant format`
}

// A nested use: the call's value is used, so append is legal there.
func nested(buf []byte) int {
	return len(fmt.Appendf(buf, "abc")) // want `fmt\.Appendf on a verbless constant format`
}

// --- ADVISORY: reported but NOT fixed (not provably clean to rewrite) ---

type ByteBuf []byte

// A NAMED []byte destination: append would return the NAMED type where
// fmt.Appendf returns []byte.
func namedDest(buf ByteBuf) []byte {
	return fmt.Appendf(buf, "x") // want `fmt\.Appendf on a verbless constant format`
}

// An untyped nil destination has no []byte type for append to infer.
func nilDest() []byte {
	return fmt.Appendf(nil, "x") // want `fmt\.Appendf on a verbless constant format`
}

// An empty format is still bit-identical, but append(buf, ""...) is noise.
func empty(buf []byte) []byte {
	return fmt.Appendf(buf, "") // want `fmt\.Appendf on a verbless constant format`
}

// The builtin append cannot stand as a bare statement or under go/defer.
func discarded(buf []byte) {
	fmt.Appendf(buf, "x")       // want `fmt\.Appendf on a verbless constant format`
	go fmt.Appendf(buf, "x")    // want `fmt\.Appendf on a verbless constant format`
	defer fmt.Appendf(buf, "x") // want `fmt\.Appendf on a verbless constant format`
}

// A comment inside the rewritten scaffolding would be destroyed.
func commented(buf []byte) []byte {
	return fmt.Appendf(buf, "x" /* keep me */) // want `fmt\.Appendf on a verbless constant format`
}

// --- NEGATIVES: not reported ---

// Any '%' byte makes fmt transform the bytes — never matched.
func percent(buf []byte) []byte {
	buf = fmt.Appendf(buf, "100%% done")
	buf = fmt.Appendf(buf, "50%")
	return fmt.Appendf(buf, "%d")
}

// A verb with operands is a different shape (PS5015/PS2141 territory).
func withArgs(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "n=%d", n)
}

// A non-literal format proves nothing.
func varFormat(buf []byte) []byte {
	format := "static"
	return fmt.Appendf(buf, format)
}

// A concatenation is not a string literal.
func concat(buf []byte) []byte {
	return fmt.Appendf(buf, "a"+"b")
}

// A spread call passes an unknown number of operands.
func spread(buf []byte, args ...any) []byte {
	return fmt.Appendf(buf, "static", args...)
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Appendf(b []byte, format string, args ...any) []byte { return b }

func shadowed(buf []byte) []byte {
	fmt := fakeFmt{}
	return fmt.Appendf(buf, "static")
}
