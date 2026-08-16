package ps5049

import "fmt"

type buffer []byte

func mk() []byte { return nil }

func other(n int) string { return "" }

// --- fixable: unnamed []byte dst, package-level fmt.Sprint* spread ---

func sprintfArgs(dst []byte, id int) []byte {
	return append(dst, fmt.Sprintf("user=%d", id)...) // want `fmt\.Appendf\(dst, \.\.\.\) formats into dst directly`
}

func sprintVerbless(dst []byte, a, b int) []byte {
	return append(dst, fmt.Sprint(a, b)...) // want `fmt\.Append\(dst, \.\.\.\) formats into dst directly`
}

func sprintlnTwin(dst []byte, a, b string) []byte {
	return append(dst, fmt.Sprintln(a, b)...) // want `fmt\.Appendln\(dst, \.\.\.\) formats into dst directly`
}

func sprintfFormatOnly(dst []byte) []byte {
	return append(dst, fmt.Sprintf("literal")...) // want `fmt\.Appendf\(dst, \.\.\.\) formats into dst directly`
}

// The inner spread (args...) is preserved verbatim.
func innerEllipsis(dst []byte, format string, args ...any) []byte {
	return append(dst, fmt.Sprintf(format, args...)...) // want `fmt\.Appendf\(dst, \.\.\.\) formats into dst directly`
}

func sprintZeroArg(dst []byte) []byte {
	return append(dst, fmt.Sprint()...) // want `fmt\.Append\(dst, \.\.\.\) formats into dst directly`
}

func sideEffectDst(x int) []byte {
	return append(mk(), fmt.Sprint(x)...) // want `fmt\.Append\(dst, \.\.\.\) formats into dst directly`
}

// --- advisory: reported, no fix ---

// A named byte-slice destination: append returns buffer, fmt.Append* returns
// []byte, so a fix would change the static type.
func namedDst(dst buffer, id int) buffer {
	return append(dst, fmt.Sprintf("%d", id)...) // want `fmt\.Appendf\(dst, \.\.\.\) formats into dst directly`
}

func commentInside(dst []byte, x int) []byte {
	return append(dst /* keep */, fmt.Sprint(x)...) // want `fmt\.Append\(dst, \.\.\.\) formats into dst directly`
}

// --- negatives: silent ---

// No spread: appends a single string element to a []string.
func noSpread(s []string, id int) []string {
	return append(s, fmt.Sprintf("%d", id))
}

// Parenthesized inner call is conservatively skipped.
func parenInner(dst []byte, id int) []byte {
	return append(dst, (fmt.Sprintf("%d", id))...)
}

// The spread argument is not an fmt formatter.
func notFmt(dst []byte, n int) []byte {
	return append(dst, other(n)...)
}
